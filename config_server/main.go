// config_server is a small config distribution server: it accepts rendered
// config files over HTTP (gatewayctl push-http) and serves the latest
// version back out (config-fetcher polling inside the Envoy container).
//
// This decouples gatewayctl from the server's filesystem - the server
// keeps its own storage and can run anywhere reachable over HTTP, not just
// colocated on the same disk as gatewayctl.
package main

import (
	"crypto/subtle"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	storageDir string
	apiToken   string
)

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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func logf(format string, args ...any) {
	log.Printf("[%s] {config_server} %s",
		time.Now().Format("2006-01-02 15:04:05,000"), fmt.Sprintf(format, args...))
}

// checkAuth gates /config/* on API_TOKEN, which main() requires to be set
// before the server starts - so it's always on, not opt-in. /healthz is
// never gated, so liveness probes don't need the token.
func checkAuth(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/config/") {
		return true
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	token := ""
	if strings.HasPrefix(header, prefix) {
		token = header[len(prefix):]
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(apiToken)) == 1
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if !checkAuth(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	filename := filepath.Base(strings.TrimPrefix(r.URL.Path, "/config/"))
	localPath := filepath.Join(storageDir, filename)

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(localPath)
		if err != nil {
			logf("WARNING - Download failed, %s not found in %s", filename, storageDir)
			http.NotFound(w, r)
			return
		}
		logf("INFO - Downloaded %s successfully (%d bytes) by %s", filename, len(data), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)

	case http.MethodPost:
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "invalid multipart form", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing 'file' field", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := atomicWrite(localPath, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		logf("INFO - Uploaded %s successfully (%d bytes) by %s", filename, len(data), r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","filename":%q,"bytes":%d}`, filename, len(data))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func main() {
	log.SetFlags(0)

	storageDir = envOr("CONFIG_SERVER_STORAGE_DIR", filepath.Join(".", "storage"))
	apiToken = os.Getenv("API_TOKEN")
	if apiToken == "" {
		log.Fatal("API_TOKEN is required and was not set")
	}
	port := envOr("PORT", "8090")

	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		log.Fatalf("creating %s: %v", storageDir, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/config/", handleConfig)
	mux.HandleFunc("/healthz", handleHealthz)

	addr := ":" + port
	logf("INFO - Serving on %s, storage=%s", addr, storageDir)
	if _, err := strconv.Atoi(port); err != nil {
		log.Fatalf("invalid PORT: %s", port)
	}
	log.Fatal(http.ListenAndServe(addr, mux))
}
