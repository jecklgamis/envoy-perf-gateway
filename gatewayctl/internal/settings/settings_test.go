package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadMissingFileReturnsZeroValueNoError(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != (Settings{}) {
		t.Errorf("got %+v, want zero value", s)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	want := Settings{
		Mode: "http",
		HTTP: HTTPSettings{ServerURL: "http://localhost:8090", APIToken: "secret"},
		S3:   S3Settings{Bucket: "my-bucket", Prefix: "envoy-perf-gateway/"},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSaveCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "config.yaml")
	if err := Save(path, Settings{Mode: "s3"}); err != nil {
		t.Fatalf("Save into a non-existent parent dir: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Mode != "s3" {
		t.Errorf("got mode %q, want s3", got.Mode)
	}
}

func TestSaveWritesRestrictivePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file permissions don't apply on Windows")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := Save(path, Settings{HTTP: HTTPSettings{APIToken: "secret"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("got permissions %o, want 0600 (api_token may hold a secret)", perm)
	}
}

func TestDefaultPathEndsInGatewayctlConfigYaml(t *testing.T) {
	p := DefaultPath()
	if filepath.Base(p) != "config.yaml" || filepath.Base(filepath.Dir(p)) != "gatewayctl" {
		t.Errorf("DefaultPath() = %q, want .../gatewayctl/config.yaml", p)
	}
}
