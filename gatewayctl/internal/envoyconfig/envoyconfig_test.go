package envoyconfig

import "testing"

func TestFaultRuntimeKeysAreNamespacedByTarget(t *testing.T) {
	abortPercent, abortStatus, delayPercent, delayDuration := FaultRuntimeKeys("httpbin")

	want := map[string]string{
		"abortPercent":  "fault.http.abort.abort_percent.httpbin",
		"abortStatus":   "fault.http.abort.http_status.httpbin",
		"delayPercent":  "fault.http.delay.delay_percent.httpbin",
		"delayDuration": "fault.http.delay.fixed_duration_ms.httpbin",
	}
	got := map[string]string{
		"abortPercent": abortPercent, "abortStatus": abortStatus,
		"delayPercent": delayPercent, "delayDuration": delayDuration,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q, want %q", k, got[k], w)
		}
	}
}

func TestFaultRuntimeKeysNeverCollideAcrossTargets(t *testing.T) {
	a1, a2, a3, a4 := FaultRuntimeKeys("httpbin")
	b1, b2, b3, b4 := FaultRuntimeKeys("default_app")

	for i, pair := range [][2]string{{a1, b1}, {a2, b2}, {a3, b3}, {a4, b4}} {
		if pair[0] == pair[1] {
			t.Errorf("key %d identical for different targets: %q", i, pair[0])
		}
	}
}

func TestFaultPerRouteDefaultsToZeroPercentNoOp(t *testing.T) {
	entry := FaultPerRoute("httpbin")
	cfg, ok := entry["envoy.filters.http.fault"].(FaultTypedConfig)
	if !ok {
		t.Fatalf("entry[\"envoy.filters.http.fault\"] is %T, want FaultTypedConfig", entry["envoy.filters.http.fault"])
	}
	if cfg.Abort.Percentage.Numerator != 0 || cfg.Delay.Percentage.Numerator != 0 {
		t.Errorf("expected 0%% abort/delay by default, got abort=%d delay=%d",
			cfg.Abort.Percentage.Numerator, cfg.Delay.Percentage.Numerator)
	}
}

func TestFaultPerRouteWiresRuntimeKeysForItsOwnTarget(t *testing.T) {
	entry := FaultPerRoute("httpbin")
	cfg := entry["envoy.filters.http.fault"].(FaultTypedConfig)

	abortPercent, abortStatus, delayPercent, delayDuration := FaultRuntimeKeys("httpbin")
	if cfg.AbortPercentRuntime != abortPercent ||
		cfg.AbortHTTPStatusRuntime != abortStatus ||
		cfg.DelayPercentRuntime != delayPercent ||
		cfg.DelayDurationRuntime != delayDuration {
		t.Errorf("runtime keys don't match FaultRuntimeKeys(\"httpbin\"): %+v", cfg)
	}
}
