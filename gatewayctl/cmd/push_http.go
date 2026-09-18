package cmd

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	phServerURL string
	phAPIToken  string
)

var pushHTTPCmd = &cobra.Command{
	Use:   "push-http",
	Short: "Upload rendered cds.yaml and lds.yaml to a running config_server over HTTP",
	Long: `Upload rendered cds.yaml and lds.yaml to a running config_server
over HTTP. Use this whenever the server isn't colocated with gatewayctl on
the same filesystem - e.g. it's deployed separately from wherever you
run the CLI. Regenerates first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := regenerate(); err != nil {
			return err
		}
		return pushHTTPFiles(phServerURL, phAPIToken)
	},
}

// pushHTTPFiles uploads the already-rendered cds.yaml/lds.yaml. Callers
// that need a fresh render first (the push-http command, invoked
// standalone) call regenerate() themselves before this.
func pushHTTPFiles(serverURL, apiToken string) error {
	serverURL = strings.TrimRight(serverURL, "/")
	for _, filename := range []string{"cds.yaml", "lds.yaml"} {
		localPath := filepath.Join(renderedDir, filename)
		url := fmt.Sprintf("%s/config/%s", serverURL, filename)
		if err := uploadFile(url, localPath, apiToken); err != nil {
			return fmt.Errorf("uploading %s: %w", localPath, err)
		}
		fmt.Printf("Uploaded %s -> %s\n", localPath, url)
	}
	return nil
}

func uploadFile(url, localPath, apiToken string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, string(body))
	}
	return nil
}

func init() {
	pushHTTPCmd.Flags().StringVar(&phServerURL, "server-url", envOr("CONFIG_SERVER_URL", "http://localhost:8090"),
		"Base URL of a running config_server.")
	pushHTTPCmd.Flags().StringVar(&phAPIToken, "api-token", os.Getenv("CONFIG_SERVER_API_TOKEN"),
		"Sent as 'Authorization: Bearer <token>'. Required if the server was started with API_TOKEN set.")
	rootCmd.AddCommand(pushHTTPCmd)
}
