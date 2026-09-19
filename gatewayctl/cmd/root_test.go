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
