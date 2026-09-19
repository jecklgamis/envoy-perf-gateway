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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gopkg.in/yaml.v3"
)

type source interface {
	fetch(filename string) ([]byte, error)
}

type httpSource struct {
	baseURL string
	token   string
	client  *http.Client
}

func (s *httpSource) fetch(filename string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, s.baseURL+"/config/"+filename, nil)
	if err != nil {
		return nil, err
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

type s3Source struct {
	bucket string
	prefix string
	client *s3.Client
}

func (s *s3Source) fetch(filename string) ([]byte, error) {
	key := s.prefix + filename
	out, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
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
			client: s3.NewFromConfig(cfg),
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

// expandRuntimeLayer decodes a runtime.yaml manifest and reconciles dir so
// it holds exactly one file per key, named after the key with the value as
// its content. Each write is atomic (temp file + rename), matching
// atomicWrite elsewhere in this file, and a key removed from the manifest
// (e.g. after `gatewayctl fault reset`) has its file removed here too, so
// the disk_layer doesn't keep applying a stale override.
func expandRuntimeLayer(dir string, content []byte) error {
	var desired map[string]string
	if err := yaml.Unmarshal(content, &desired); err != nil {
		return fmt.Errorf("decoding %s: %w", runtimeManifestFilename, err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	existing, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	stale := make(map[string]bool, len(existing))
	for _, entry := range existing {
		if !entry.IsDir() {
			stale[entry.Name()] = true
		}
	}

	for key, value := range desired {
		if err := atomicWrite(filepath.Join(dir, key), []byte(value)); err != nil {
			return fmt.Errorf("writing runtime key %s: %w", key, err)
		}
		delete(stale, key)
	}

	for key := range stale {
		if err := os.Remove(filepath.Join(dir, key)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing stale runtime key %s: %w", key, err)
		}
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

	logMsg("INFO", "Polling %s every %ss for %v -> %s", sourceKind, pollIntervalStr, files, targetDir)

	lastHash := map[string]string{}
	for {
		for i, filename := range files {
			content, err := src.fetch(filename)
			if err != nil {
				logMsg("WARNING", "Fetch failed for %s: %v", filename, err)
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
				runtimeDir := filepath.Join(targetDir, "runtime", "current")
				if writeErr = expandRuntimeLayer(runtimeDir, content); writeErr == nil {
					logMsg("INFO", "Fetched %s successfully, updated %s (%d bytes)", filename, runtimeDir, len(content))
				}
			} else {
				targetPath := filepath.Join(targetDir, filename)
				if writeErr = atomicWrite(targetPath, content); writeErr == nil {
					logMsg("INFO", "Fetched %s successfully, updated %s (%d bytes)", filename, targetPath, len(content))
				}
			}
			if writeErr != nil {
				logMsg("WARNING", "Failed to apply %s: %v", filename, writeErr)
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
		time.Sleep(time.Duration(pollInterval * float64(time.Second)))
	}
}
