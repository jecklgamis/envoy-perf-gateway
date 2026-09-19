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

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
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
		switch mode := resolveMode(); mode {
		case "":
			return fmt.Errorf("no mode configured (gatewayctl config set mode http|s3, or CONFIG_SOURCE_KIND)")
		case "http":
			return fetchHTTPFiles(resolveServerURL(), resolveAPIToken())
		case "s3":
			bucket := resolveS3Bucket()
			if bucket == "" {
				return fmt.Errorf("mode=s3 requires CONFIG_S3_BUCKET or settings file s3.bucket to be set")
			}
			return fetchS3Files(bucket, resolveS3Prefix())
		default:
			return fmt.Errorf("unsupported mode: %s (want \"http\" or \"s3\")", mode)
		}
	},
}

func fetchHTTPFiles(serverURL, apiToken string) error {
	serverURL = strings.TrimRight(serverURL, "/")
	client := &http.Client{Timeout: 10 * time.Second}

	for _, filename := range []string{"cds.yaml", "lds.yaml", "runtime.yaml"} {
		url := fmt.Sprintf("%s/config/%s", serverURL, filename)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		if apiToken != "" {
			req.Header.Set("Authorization", "Bearer "+apiToken)
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("fetching %s: %w", url, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", url, readErr)
		}
		if resp.StatusCode == http.StatusNotFound {
			fmt.Printf("%s not found on the config server yet (nothing pushed) - skipping\n", filename)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetching %s: %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
		}
		if err := writeLocal(filename, body); err != nil {
			return err
		}
	}
	return nil
}

func fetchS3Files(bucket, prefix string) error {
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	client := s3.NewFromConfig(cfg)
	prefix = strings.TrimLeft(prefix, "/")

	for _, filename := range []string{"cds.yaml", "lds.yaml", "runtime.yaml"} {
		key := prefix + filename
		out, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
		if err != nil {
			if strings.Contains(err.Error(), "NoSuchKey") {
				fmt.Printf("%s not found in s3://%s/%s yet (nothing pushed) - skipping\n", filename, bucket, key)
				continue
			}
			return fmt.Errorf("fetching s3://%s/%s: %w", bucket, key, err)
		}
		body, err := io.ReadAll(out.Body)
		out.Body.Close()
		if err != nil {
			return fmt.Errorf("reading s3://%s/%s: %w", bucket, key, err)
		}
		if err := writeLocal(filename, body); err != nil {
			return err
		}
	}
	return nil
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
