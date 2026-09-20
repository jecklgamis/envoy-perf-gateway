package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch cds.yaml, lds.yaml, and runtime.yaml from the config server or S3 into --rendered-dir",
	Long: `Fetch cds.yaml, lds.yaml, and runtime.yaml from the config server or S3
into --rendered-dir - the same source the in-container fetcher polls.

This is the ground truth of what's actually live, which is not
necessarily the same as what's in local values.yaml: a push can fail
silently, or values.yaml here may not be the one that was last used to
push. Uses whichever mode is configured (gatewayctl config set mode
http|s3, or CONFIG_SOURCE_KIND), the same way push-http/push-s3 do.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := os.MkdirAll(renderedDir, 0o755); err != nil {
			return err
		}
		for _, filename := range []string{"cds.yaml", "lds.yaml", "runtime.yaml"} {
			body, found, err := fetchRemoteFile(filename)
			if err != nil {
				return err
			}
			if !found {
				fmt.Printf("%s not found on the remote source yet (nothing pushed) - skipping\n", filename)
				continue
			}
			if err := writeLocal(filename, body); err != nil {
				return err
			}
		}
		return nil
	},
}

// fetchRemoteFile downloads filename from whichever mode is configured.
// found is false (with a nil error) when the remote genuinely has nothing
// at that name yet - not pushed, not a failure - callers decide how to
// report that; any other problem (bad token, wrong mode, network error)
// comes back as a non-nil error.
func fetchRemoteFile(filename string) ([]byte, bool, error) {
	switch mode := resolveMode(); mode {
	case "":
		return nil, false, fmt.Errorf("no mode configured (gatewayctl config set mode http|s3, or CONFIG_SOURCE_KIND)")
	case "http":
		return fetchHTTPFile(resolveServerURL(), resolveAPIToken(), filename)
	case "s3":
		bucket := resolveS3Bucket()
		if bucket == "" {
			return nil, false, fmt.Errorf("mode=s3 requires CONFIG_S3_BUCKET or settings file s3.bucket to be set")
		}
		return fetchS3File(bucket, resolveS3Prefix(), filename)
	default:
		return nil, false, fmt.Errorf("unsupported mode: %s (want \"http\" or \"s3\")", mode)
	}
}

func fetchHTTPFile(serverURL, apiToken, filename string) ([]byte, bool, error) {
	url := fmt.Sprintf("%s/config/%s", strings.TrimRight(serverURL, "/"), filename)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("reading %s: %w", url, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("fetching %s: %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
	}
	return body, true, nil
}

func fetchS3File(bucket, prefix, filename string) ([]byte, bool, error) {
	ctx := context.Background()
	client, err := newS3Client(ctx)
	if err != nil {
		return nil, false, err
	}
	key := strings.TrimLeft(prefix, "/") + filename

	out, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchKey") {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("fetching s3://%s/%s: %w", bucket, key, err)
	}
	defer out.Body.Close()
	body, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, false, fmt.Errorf("reading s3://%s/%s: %w", bucket, key, err)
	}
	return body, true, nil
}

func writeLocal(filename string, body []byte) error {
	localPath := filepath.Join(renderedDir, filename)
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		return err
	}
	fmt.Printf("Fetched %s -> %s (%d bytes)\n", filename, localPath, len(body))
	return nil
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}
