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
	Short: "Upload rendered cds.yaml, lds.yaml, and runtime.yaml to a running config_server over HTTP",
	Long: `Upload rendered cds.yaml, lds.yaml, and runtime.yaml to a running config_server
over HTTP. Use this whenever the server isn't colocated with gatewayctl on
the same filesystem - e.g. it's deployed separately from wherever you
run the CLI. Regenerates first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := regenerate(); err != nil {
			return err
		}
		serverURL := phServerURL
		if !cmd.Flags().Changed("server-url") {
			serverURL = resolveServerURL()
		}
		apiToken := phAPIToken
		if !cmd.Flags().Changed("api-token") {
			apiToken = resolveAPIToken()
		}
		return pushHTTPFiles(serverURL, apiToken)
	},
}

// pushHTTPFiles uploads the already-rendered cds.yaml/lds.yaml/runtime.yaml. Callers
// that need a fresh render first (the push-http command, invoked
// standalone) call regenerate() themselves before this.
func pushHTTPFiles(serverURL, apiToken string) error {
	serverURL = strings.TrimRight(serverURL, "/")
	for _, filename := range []string{"cds.yaml", "lds.yaml", "runtime.yaml"} {
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
	// Static defaults shown in --help; actual resolution (CLI flag > env
	// var > settings file > this default) happens in RunE via
	// resolveServerURL()/resolveAPIToken(), since the settings file isn't
	// loaded yet when init() runs.
	pushHTTPCmd.Flags().StringVar(&phServerURL, "server-url", "http://localhost:8090",
		"Base URL of a running config_server. Falls back to CONFIG_SERVER_URL, then the settings file's http.server_url.")
	pushHTTPCmd.Flags().StringVar(&phAPIToken, "api-token", "",
		"Sent as 'Authorization: Bearer <token>'. Falls back to CONFIG_SERVER_API_TOKEN, then the settings file's http.api_token.")
	rootCmd.AddCommand(pushHTTPCmd)
}
