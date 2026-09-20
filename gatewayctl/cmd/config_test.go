package cmd

import (
	"testing"

	"github.com/jecklgamis/envoy-perf-gateway/gatewayctl/internal/settings"
)

func TestSettingsGetReturnsRealValueForEachKey(t *testing.T) {
	s := settings.Settings{
		Mode: "http",
		HTTP: settings.HTTPSettings{ServerURL: "http://localhost:8090", APIToken: "secret"},
		S3:   settings.S3Settings{Bucket: "my-bucket", Prefix: "envoy-perf-gateway/"},
	}
	want := map[string]string{
		"mode":            "http",
		"http.server-url": "http://localhost:8090",
		"http.api-token":  "secret",
		"s3.bucket":       "my-bucket",
		"s3.prefix":       "envoy-perf-gateway/",
	}
	for key, wantValue := range want {
		got, ok := settingsGet(s, key)
		if !ok {
			t.Errorf("settingsGet(%q) reported unknown key", key)
		}
		if got != wantValue {
			t.Errorf("settingsGet(%q) = %q, want %q", key, got, wantValue)
		}
	}
}

func TestSettingsGetUnknownKey(t *testing.T) {
	if _, ok := settingsGet(settings.Settings{}, "not-a-real-key"); ok {
		t.Error("settingsGet reported an unknown key as found")
	}
}

func TestSettingsGetHasNoRedactionLogicOfItsOwn(t *testing.T) {
	// settingsGet is a dumb accessor - whether its output is the real
	// value or a placeholder depends entirely on whether the caller
	// passes it appSettings (configGetCmd's direct-key-lookup path, which
	// intentionally shows the real value) or settings.Redacted(appSettings)
	// (the "print everything" dump, and the `set` confirmation echo).
	redacted := settings.Redacted(settings.Settings{HTTP: settings.HTTPSettings{APIToken: "secret"}})
	got, _ := settingsGet(redacted, "http.api-token")
	if got == "secret" {
		t.Fatal("test setup broken: settings.Redacted did not redact")
	}
	if got != "********" {
		t.Errorf("settingsGet on an already-redacted Settings returned %q, want the placeholder", got)
	}
}
