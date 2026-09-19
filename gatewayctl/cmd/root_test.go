package cmd

import "testing"

func TestValidateName(t *testing.T) {
	valid := []string{"httpbin", "default_app", "svc-a", "svc_b-2", "A1"}
	for _, name := range valid {
		if err := validateName("name", name); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", name, err)
		}
	}

	// Anything a filepath.Join could turn into a traversal outside the
	// fetcher's data directory (see fetcher's expandRuntimeLayer) must be
	// rejected here before it ever reaches a runtime key.
	invalid := []string{
		"../evil",
		"../../etc/cron.d/evil",
		"a/b",
		"a b",
		"",
		".",
		"..",
		"name!",
		"name.with.dots",
	}
	for _, name := range invalid {
		if err := validateName("name", name); err == nil {
			t.Errorf("validateName(%q) = nil, want an error", name)
		}
	}
}

func TestValidatePort(t *testing.T) {
	for _, p := range []int{1, 80, 443, 65535} {
		if err := validatePort("port", p); err != nil {
			t.Errorf("validatePort(%d) = %v, want nil", p, err)
		}
	}
	for _, p := range []int{0, -1, 65536, 100000} {
		if err := validatePort("port", p); err == nil {
			t.Errorf("validatePort(%d) = nil, want an error", p)
		}
	}
}

func TestValidatePercent(t *testing.T) {
	for _, p := range []int{0, 1, 50, 100} {
		if err := validatePercent("percent", p); err != nil {
			t.Errorf("validatePercent(%d) = %v, want nil", p, err)
		}
	}
	for _, p := range []int{-1, 101, 1000} {
		if err := validatePercent("percent", p); err == nil {
			t.Errorf("validatePercent(%d) = nil, want an error", p)
		}
	}
}

func TestValidateHTTPStatus(t *testing.T) {
	for _, s := range []int{100, 200, 404, 503, 599} {
		if err := validateHTTPStatus("status", s); err != nil {
			t.Errorf("validateHTTPStatus(%d) = %v, want nil", s, err)
		}
	}
	for _, s := range []int{99, 0, -1, 600, 5000} {
		if err := validateHTTPStatus("status", s); err == nil {
			t.Errorf("validateHTTPStatus(%d) = nil, want an error", s)
		}
	}
}

func TestValidateNonNegative(t *testing.T) {
	for _, n := range []int{0, 1, 2000} {
		if err := validateNonNegative("duration-ms", n); err != nil {
			t.Errorf("validateNonNegative(%d) = %v, want nil", n, err)
		}
	}
	if err := validateNonNegative("duration-ms", -1); err == nil {
		t.Error("validateNonNegative(-1) = nil, want an error")
	}
}

func TestValidateDuration(t *testing.T) {
	for _, d := range []string{"1ms", "500ms", "5s", "2m", "1h"} {
		if err := validateDuration("timeout", d); err != nil {
			t.Errorf("validateDuration(%q) = %v, want nil", d, err)
		}
	}
	invalid := []string{"0s", "-5s", "5", "5 seconds", "", "5sec"}
	for _, d := range invalid {
		if err := validateDuration("timeout", d); err == nil {
			t.Errorf("validateDuration(%q) = nil, want an error", d)
		}
	}
}
