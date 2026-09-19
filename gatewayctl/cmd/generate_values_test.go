package cmd

import (
	"testing"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/config"
)

func TestParseRemoteFaultsAllFourKeyTypes(t *testing.T) {
	body := []byte(`
fault.http.abort.abort_percent.httpbin: "100"
fault.http.abort.http_status.httpbin: "503"
fault.http.delay.delay_percent.default_app: "20"
fault.http.delay.fixed_duration_ms.default_app: "2000"
`)
	got, err := parseRemoteFaults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]config.FaultSpec{
		"httpbin":     {AbortPercent: 100, AbortStatus: 503},
		"default_app": {DelayPercent: 20, DelayDurationMs: 2000},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d targets, want %d: %+v", len(got), len(want), got)
	}
	for target, wantSpec := range want {
		gotSpec, ok := got[target]
		if !ok {
			t.Fatalf("missing target %q in %+v", target, got)
		}
		if gotSpec != wantSpec {
			t.Errorf("target %q: got %+v, want %+v", target, gotSpec, wantSpec)
		}
	}
}

func TestParseRemoteFaultsMergesBothDimensionsForSameTarget(t *testing.T) {
	body := []byte(`
fault.http.abort.abort_percent.httpbin: "50"
fault.http.delay.delay_percent.httpbin: "50"
fault.http.delay.fixed_duration_ms.httpbin: "1000"
`)
	got, err := parseRemoteFaults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := config.FaultSpec{AbortPercent: 50, DelayPercent: 50, DelayDurationMs: 1000}
	if got["httpbin"] != want {
		t.Errorf("got %+v, want %+v", got["httpbin"], want)
	}
}

func TestParseRemoteFaultsEmptyManifest(t *testing.T) {
	got, err := parseRemoteFaults([]byte("{}\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d entries for an empty manifest, want 0: %+v", len(got), got)
	}
}

func TestParseRemoteFaultsUnknownKeysIgnored(t *testing.T) {
	body := []byte(`
some.other.runtime.key: "1"
fault.http.abort.abort_percent.httpbin: "10"
`)
	got, err := parseRemoteFaults(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got["httpbin"].AbortPercent != 10 {
		t.Errorf("got %+v, want only httpbin.AbortPercent=10", got)
	}
}

func TestParseRemoteFaultsRejectsNonIntegerValue(t *testing.T) {
	body := []byte(`fault.http.abort.abort_percent.httpbin: "not-a-number"`)
	if _, err := parseRemoteFaults(body); err == nil {
		t.Fatal("expected an error for a non-integer runtime value, got nil")
	}
}
