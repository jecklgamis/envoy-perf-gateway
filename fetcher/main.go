// config-fetcher polls a remote source for cds.yaml/lds.yaml and atomically
// writes any changes into the directory Envoy watches via inotify.
//
// Runs INSIDE the Envoy container so the write is native to the container's
// filesystem - this is what makes the inotify-based hot-reload actually
// fire, unlike a host-side bind-mount write on Docker Desktop for Mac.
//
// Source kind is selected with CONFIG_SOURCE_KIND=http|s3.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gopkg.in/yaml.v3"
)

// fetch returns (content, found, err). found=false, err=nil means the
// source has no content for filename yet (nothing pushed there ever) -
// distinct from a real error, so the poll loop can treat "nothing to sync
// yet" as a clean, convergence-counting outcome rather than a failure to
// retry.
type source interface {
	fetch(filename string) (content []byte, found bool, err error)
}

type httpSource struct {
	baseURL string
	token   string
	client  *http.Client
}

func (s *httpSource) fetch(filename string) ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, s.baseURL+"/config/"+filename, nil)
	if err != nil {
		return nil, false, err
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	content, err := io.ReadAll(resp.Body)
	return content, true, err
}

type s3Source struct {
	bucket string
	prefix string
	client *s3.Client
}

func (s *s3Source) fetch(filename string) ([]byte, bool, error) {
	key := s.prefix + filename
	out, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchKey") {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer out.Body.Close()
	content, err := io.ReadAll(out.Body)
	return content, true, err
}

func buildSource(kind string) (source, error) {
	switch kind {
	case "http":
		baseURL := os.Getenv("CONFIG_SOURCE_URL")
		if baseURL == "" {
			return nil, fmt.Errorf("CONFIG_SOURCE_URL is required for CONFIG_SOURCE_KIND=http")
		}
		return &httpSource{
			baseURL: strings.TrimRight(baseURL, "/"),
			token:   os.Getenv("CONFIG_API_TOKEN"),
			client:  &http.Client{Timeout: 5 * time.Second},
		}, nil
	case "s3":
		bucket := os.Getenv("CONFIG_S3_BUCKET")
		if bucket == "" {
			return nil, fmt.Errorf("CONFIG_S3_BUCKET is required for CONFIG_SOURCE_KIND=s3")
		}
		cfg, err := awsconfig.LoadDefaultConfig(context.Background())
		if err != nil {
			return nil, err
		}
		return &s3Source{
			bucket: bucket,
			prefix: strings.TrimLeft(os.Getenv("CONFIG_S3_PREFIX"), "/"),
			client: s3.NewFromConfig(cfg, func(o *s3.Options) {
				// Only needed against an S3-compatible endpoint (e.g. MinIO
				// in the integration test) via AWS_ENDPOINT_URL_S3 - real
				// AWS S3 doesn't need or want this.
				if os.Getenv("S3_FORCE_PATH_STYLE") == "true" {
					o.UsePathStyle = true
				}
			}),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported CONFIG_SOURCE_KIND: %s", kind)
	}
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// runtimeManifestFilename is the one file gatewayctl renders/pushes for
// fault injection - a flat map of Envoy runtime key -> value. Unlike
// cds.yaml/lds.yaml, Envoy doesn't read it directly: its layered_runtime
// disk_layer (config/envoy.yaml) expects one regular file per key inside a
// directory, so expandRuntimeLayer below fans this single manifest out
// into that directory instead of writing it as-is.
const runtimeManifestFilename = "runtime.yaml"

// expandRuntimeLayer decodes a runtime.yaml manifest into a fresh
// "data-<digest>" directory under runtimeRoot (one file per key), then
// atomically swaps runtimeRoot/current - a symlink, per Envoy's disk_layer
// contract - to point at it, and removes the previous data directory.
//
// This isn't the same atomic-rename trick used elsewhere in this file for
// cds.yaml/lds.yaml: Envoy's disk_layer only reloads when the symlink at
// its configured symlink_root is itself replaced (the same scheme
// Kubernetes uses for ConfigMap volumes), not when files inside an
// already-referenced directory change in place - a plain directory with
// files rewritten in it, which is what an earlier version of this function
// did, is silently invisible to it (confirmed via /runtime on the admin
// API: the layer registers but "entries" never populates).
func expandRuntimeLayer(runtimeRoot string, content []byte) error {
	var desired map[string]string
	if err := yaml.Unmarshal(content, &desired); err != nil {
		return fmt.Errorf("decoding %s: %w", runtimeManifestFilename, err)
	}

	sum := sha256.Sum256(content)
	dataDir := filepath.Join(runtimeRoot, "data-"+hex.EncodeToString(sum[:])[:16])
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	for key, value := range desired {
		// gatewayctl validates --name/--target before a key ever reaches
		// this manifest, but this process runs as root and this content
		// came over the network, so refuse anything that isn't a bare
		// filename on its own merits too - a key containing a path
		// separator or ".." must never be allowed to write outside
		// dataDir via filepath.Join's cleaning.
		if key == "" || key != filepath.Base(key) || key == "." || key == ".." {
			return fmt.Errorf("refusing unsafe runtime key %q", key)
		}
		if err := atomicWrite(filepath.Join(dataDir, key), []byte(value)); err != nil {
			return fmt.Errorf("writing runtime key %s: %w", key, err)
		}
	}

	symlinkPath := filepath.Join(runtimeRoot, "current")
	tmpLink := symlinkPath + ".tmp"
	os.Remove(tmpLink)
	if err := os.Symlink(dataDir, tmpLink); err != nil {
		return fmt.Errorf("creating %s: %w", tmpLink, err)
	}
	previousTarget, _ := os.Readlink(symlinkPath)
	if err := os.Rename(tmpLink, symlinkPath); err != nil {
		return fmt.Errorf("swapping %s: %w", symlinkPath, err)
	}

	if previousTarget != "" && previousTarget != dataDir {
		os.RemoveAll(previousTarget)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func logMsg(level, format string, args ...any) {
	log.Printf("[%s] {config-fetcher} %s - %s",
		time.Now().Format("2006-01-02 15:04:05,000"), level, fmt.Sprintf(format, args...))
}

func main() {
	log.SetFlags(0)

	targetDir := envOr("CONFIG_TARGET_DIR", "/etc/envoy/dynamic")
	sourceKind := envOr("CONFIG_SOURCE_KIND", "http")
	pollIntervalStr := envOr("CONFIG_POLL_INTERVAL_SECONDS", "15")
	pollInterval, err := strconv.ParseFloat(pollIntervalStr, 64)
	if err != nil {
		log.Fatalf("invalid CONFIG_POLL_INTERVAL_SECONDS: %v", err)
	}
	files := strings.Split(envOr("CONFIG_FILES", "cds.yaml,lds.yaml,runtime.yaml"), ",")

	if info, err := os.Stat(targetDir); err != nil || !info.IsDir() {
		log.Fatalf("%s does not exist", targetDir)
	}

	src, err := buildSource(sourceKind)
	if err != nil {
		log.Fatal(err)
	}

	// Serves the container's readinessProbe (see
	// charts/envoy-perf-gateway/templates/deployment.yaml) - deliberately
	// the fetcher, not Envoy, since Envoy's own /ready goes live using
	// whatever's baked into the image immediately at boot (its
	// dynamic_resources are file-based, so there's always something local
	// to read), well before this process has ever synced the real config
	// source. A single consistent target for the probe - one HTTP call to
	// the thing that actually knows whether a real sync has happened - is
	// simpler than a probe that has to check Envoy and the fetcher
	// separately. synced flips true after the first full poll pass that
	// completes without any fetch/write error (a legitimate "nothing
	// pushed yet" counts as clean) and never flips back - a pod that has
	// already converged once shouldn't drop out of rotation just because a
	// later poll hits a transient error while still serving good config.
	var synced atomic.Bool
	readyPort := envOr("FETCHER_READY_PORT", "8081")
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
			if synced.Load() {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("synced\n"))
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not yet synced\n"))
		})
		log.Fatal(http.ListenAndServe(":"+readyPort, mux))
	}()

	logMsg("INFO", "Polling %s every %ss for %v -> %s", sourceKind, pollIntervalStr, files, targetDir)
	logMsg("INFO", "Serving readiness on :%s/ready", readyPort)

	lastHash := map[string]string{}
	for {
		cleanPass := true
		for i, filename := range files {
			content, found, err := src.fetch(filename)
			if err != nil {
				logMsg("WARNING", "Fetch failed for %s: %v", filename, err)
				cleanPass = false
				continue
			}
			if !found {
				// Nothing pushed there yet - not an error, and whatever's
				// already on disk (the image's baked-in seed, or a prior
				// poll's write) stands as-is. Still counts as a clean pass
				// for readiness purposes: this pod correctly reflects "no
				// config pushed", it just isn't stale/racing.
				logMsg("INFO", "%s: nothing pushed yet, leaving current state as-is", filename)
				continue
			}
			sum := sha256.Sum256(content)
			digest := hex.EncodeToString(sum[:])

			if lastHash[filename] == digest {
				logMsg("INFO", "Fetched %s successfully (%d bytes, unchanged)", filename, len(content))
				continue
			}

			writeErr := error(nil)
			if filename == runtimeManifestFilename {
				runtimeRoot := filepath.Join(targetDir, "runtime")
				if writeErr = expandRuntimeLayer(runtimeRoot, content); writeErr == nil {
					logMsg("INFO", "Fetched %s successfully, updated %s/current (%d bytes)", filename, runtimeRoot, len(content))
				}
			} else {
				targetPath := filepath.Join(targetDir, filename)
				if writeErr = atomicWrite(targetPath, content); writeErr == nil {
					logMsg("INFO", "Fetched %s successfully, updated %s (%d bytes)", filename, targetPath, len(content))
				}
			}
			if writeErr != nil {
				logMsg("WARNING", "Failed to apply %s: %v", filename, writeErr)
				cleanPass = false
				continue
			}
			lastHash[filename] = digest

			// CONFIG_FILES defaults to "cds.yaml,lds.yaml" - in that order
			// on purpose. Envoy reloads each file independently on its own
			// inotify event; a new listener can reference a cluster that
			// was just added, so if lds.yaml's write (and Envoy's reload
			// of it) races ahead of cds.yaml's, Envoy rejects the listener
			// with "unknown cluster" and - since we only rewrite a file
			// when its content changes - never gets another inotify event
			// to retry on, leaving it permanently stuck. A short settle
			// delay after writing a file that has more files queued behind
			// it gives Envoy's (typically sub-millisecond) cluster manager
			// update time to land first.
			if i < len(files)-1 {
				time.Sleep(300 * time.Millisecond)
			}
		}

		if cleanPass && synced.CompareAndSwap(false, true) {
			logMsg("INFO", "First full sync complete - readiness on :%s/ready now reports synced", readyPort)
		}

		time.Sleep(time.Duration(pollInterval * float64(time.Second)))
	}
}
