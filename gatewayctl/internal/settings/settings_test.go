package settings

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestRedactedHidesAPIToken(t *testing.T) {
	s := Settings{HTTP: HTTPSettings{ServerURL: "http://localhost:8090", APIToken: "secret"}}
	got := Redacted(s)
	if got.HTTP.APIToken == "secret" || got.HTTP.APIToken == "" {
		t.Errorf("got APIToken=%q, want a non-empty placeholder", got.HTTP.APIToken)
	}
}

func TestRedactedLeavesNonSecretFieldsAlone(t *testing.T) {
	s := Settings{
		Mode: "http",
		HTTP: HTTPSettings{ServerURL: "http://localhost:8090", APIToken: "secret"},
		S3:   S3Settings{Bucket: "my-bucket", Prefix: "envoy-perf-gateway/"},
	}
	got := Redacted(s)
	if got.Mode != "http" || got.HTTP.ServerURL != "http://localhost:8090" ||
		got.S3.Bucket != "my-bucket" || got.S3.Prefix != "envoy-perf-gateway/" {
		t.Errorf("Redacted changed a non-secret field: %+v", got)
	}
}

func TestRedactedLeavesUnsetSecretEmpty(t *testing.T) {
	// An unset token should still read as unset - a placeholder here
	// would make "no token configured" indistinguishable from "a token
	// is configured, but hidden", which matters for someone debugging why
	// a push is unauthenticated.
	got := Redacted(Settings{})
	if got.HTTP.APIToken != "" {
		t.Errorf("got APIToken=%q for a Settings with none set, want \"\"", got.HTTP.APIToken)
	}
}

func TestRedactedDoesNotMutateItsArgument(t *testing.T) {
	s := Settings{HTTP: HTTPSettings{APIToken: "secret"}}
	_ = Redacted(s)
	if s.HTTP.APIToken != "secret" {
		t.Errorf("Redacted mutated its argument in place: got %q, want the original preserved", s.HTTP.APIToken)
	}
}

// TestRedactedCoversNewSecretFieldsAutomatically is the actual point of
// Redacted being reflection/tag-driven rather than a hardcoded
// s.HTTP.APIToken = mask(...): tagging any new field redact:"true"
// anywhere in the struct tree, at any nesting depth, must be redacted
// with no other code changes. This defines a local type shaped like a
// hypothetical future addition to prove that, rather than relying on
// Settings itself gaining a second secret field just to test this.
func TestRedactedCoversNewSecretFieldsAutomatically(t *testing.T) {
	type deeplyNested struct {
		Credential string `redact:"true"`
	}
	type withFutureField struct {
		Mode   string
		Nested deeplyNested
	}
	v := withFutureField{Mode: "http", Nested: deeplyNested{Credential: "future-secret"}}
	redactValue(reflect.ValueOf(&v).Elem())

	if v.Nested.Credential == "future-secret" || v.Nested.Credential == "" {
		t.Errorf("a newly redact:\"true\" tagged field at nested depth was not redacted: got %q", v.Nested.Credential)
	}
	if v.Mode != "http" {
		t.Errorf("an untagged field was changed: got Mode=%q", v.Mode)
	}
}
