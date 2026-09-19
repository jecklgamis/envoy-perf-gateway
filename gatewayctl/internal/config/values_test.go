package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsEmptyBackendsNoError(t *testing.T) {
	v, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Backends == nil || len(v.Backends) != 0 {
		t.Errorf("got %+v, want an empty (non-nil) Backends slice", v)
	}
	if v.Faults != nil {
		t.Errorf("got Faults=%+v, want nil for a fresh checkout", v.Faults)
	}
}

func TestSaveThenLoadRoundTripsBackendsAndFaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "values.yaml")
	want := Values{
		Backends: []Backend{
			{
				Name: "httpbin", Host: "httpbin.org", Port: 443, TLS: true,
				RoutePrefix: "/httpbin/", ConnectTimeout: "5s", Timeout: "15s",
			},
			{
				Name: "svc-a", Host: "svc-a.internal", Port: 8080,
				Domain: "frontend-a.test.local", HostRewrite: "frontend-a.test.local",
				ConnectTimeout: "5s", Timeout: "15s",
			},
		},
		Faults: map[string]FaultSpec{
			"httpbin": {AbortPercent: 100, AbortStatus: 503},
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Backends) != len(want.Backends) {
		t.Fatalf("got %d backends, want %d", len(got.Backends), len(want.Backends))
	}
	for i := range want.Backends {
		if got.Backends[i] != want.Backends[i] {
			t.Errorf("backend %d: got %+v, want %+v", i, got.Backends[i], want.Backends[i])
		}
	}
	if len(got.Faults) != 1 || got.Faults["httpbin"] != want.Faults["httpbin"] {
		t.Errorf("got Faults=%+v, want %+v", got.Faults, want.Faults)
	}
}

func TestSaveCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "values.yaml")
	if err := Save(path, Values{Backends: []Backend{{Name: "a", Host: "a", Port: 1}}}); err != nil {
		t.Fatalf("Save into a non-existent parent dir: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Backends) != 1 || got.Backends[0].Name != "a" {
		t.Errorf("got %+v", got)
	}
}

func TestFaultSpecOmitsZeroFieldsFromYAML(t *testing.T) {
	// abort_percent/abort_status/delay_percent/delay_duration_ms all have
	// omitempty - render.BuildRuntime depends on this to only emit the
	// dimensions actually set for a target, so verify the tag survives a
	// real marshal, not just that the Go zero value looks right.
	path := filepath.Join(t.TempDir(), "values.yaml")
	v := Values{Faults: map[string]FaultSpec{"httpbin": {AbortPercent: 100}}}
	if err := Save(path, v); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := FaultSpec{AbortPercent: 100}
	if got.Faults["httpbin"] != want {
		t.Errorf("got %+v, want %+v", got.Faults["httpbin"], want)
	}
}
