// Package settings loads/saves gatewayctl's persistent YAML config file -
// the default distribution mode, config_server URL/token, and S3
// bucket/prefix - so those don't need to be re-set via env vars every
// session. Everything here is optional; a missing file just means "no
// defaults configured", not an error.
package settings

import (
	"os"
	"path/filepath"
	"reflect"

	"gopkg.in/yaml.v3"
)

// redactedPlaceholder is fixed-length and content-independent on purpose:
// a partial reveal (first/last N chars) would narrow a short real-world
// token - these are often short, human-picked dev tokens, not high-entropy
// secrets - down to very little.
const redactedPlaceholder = "********"

type HTTPSettings struct {
	ServerURL string `yaml:"server_url,omitempty"`
	// redact:"true" marks this as a secret. Redacted walks every field
	// carrying that tag (see its doc comment) - add the same tag to any
	// future secret field (an S3 credential, another service's token,
	// etc.) and it's covered everywhere Redacted is used with no other
	// code changes.
	APIToken string `yaml:"api_token,omitempty" redact:"true"`
}

type S3Settings struct {
	Bucket string `yaml:"bucket,omitempty"`
	Prefix string `yaml:"prefix,omitempty"`
}

// Settings mirrors CONFIG_SOURCE_KIND=http|s3 plus the env vars each mode
// reads (CONFIG_SERVER_URL, CONFIG_SERVER_API_TOKEN, CONFIG_S3_BUCKET,
// CONFIG_S3_PREFIX) - an env var always overrides the matching value here.
type Settings struct {
	Mode string       `yaml:"mode,omitempty"` // "", "http", or "s3"
	HTTP HTTPSettings `yaml:"http,omitempty"`
	S3   S3Settings   `yaml:"s3,omitempty"`
}

// DefaultPath is ~/.config/gatewayctl/config.yaml.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "gatewayctl", "config.yaml")
}

func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// Save writes 0600 since api_token may hold a secret.
func Save(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Redacted returns a copy of s with every string field tagged
// `redact:"true"` replaced by a fixed placeholder, if it's set at all -
// for display contexts (a full-settings dump, a "here's what got saved"
// confirmation) where the real value would otherwise end up in a
// terminal scrollback, a pasted chat log, or a screenshot. It recurses
// into nested structs, so HTTPSettings/S3Settings today - and any struct
// field added later - are covered without this function needing to know
// their shape; it only needs the tag on the field itself.
func Redacted(s Settings) Settings {
	redactValue(reflect.ValueOf(&s).Elem())
	return s
}

func redactValue(v reflect.Value) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)
		switch fv.Kind() {
		case reflect.Struct:
			redactValue(fv)
		case reflect.String:
			if field.Tag.Get("redact") == "true" && fv.String() != "" {
				fv.SetString(redactedPlaceholder)
			}
		}
	}
}
