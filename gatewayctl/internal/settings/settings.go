// Package settings loads/saves gatewayctl's persistent YAML config file -
// the default distribution mode, config_server URL/token, and S3
// bucket/prefix - so those don't need to be re-set via env vars every
// session. Everything here is optional; a missing file just means "no
// defaults configured", not an error.
package settings

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type HTTPSettings struct {
	ServerURL string `yaml:"server_url,omitempty"`
	APIToken  string `yaml:"api_token,omitempty"`
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
