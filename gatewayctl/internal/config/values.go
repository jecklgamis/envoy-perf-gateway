package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Backend mirrors one entry in values.yaml's `backends` list.
type Backend struct {
	Name           string `yaml:"name"`
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	TLS            bool   `yaml:"tls"`
	Domain         string `yaml:"domain,omitempty"`
	RoutePrefix    string `yaml:"route_prefix,omitempty"`
	ConnectTimeout string `yaml:"connect_timeout"`
	Timeout        string `yaml:"timeout"`
}

// FaultSpec is one entry in values.yaml's `faults` map, keyed by target (a
// backend name, or "default_app"). Zero fields mean "not set" (omitted
// from the rendered runtime layer, i.e. that dimension stays at baseline).
type FaultSpec struct {
	AbortPercent    int `yaml:"abort_percent,omitempty"`
	AbortStatus     int `yaml:"abort_status,omitempty"`
	DelayPercent    int `yaml:"delay_percent,omitempty"`
	DelayDurationMs int `yaml:"delay_duration_ms,omitempty"`
}

type Values struct {
	Backends []Backend            `yaml:"backends"`
	Faults   map[string]FaultSpec `yaml:"faults,omitempty"`
}

// Load reads values.yaml. A missing file is treated as an empty backend
// list (matches the Python CLI's behavior on a fresh checkout).
func Load(path string) (Values, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Values{Backends: []Backend{}}, nil
	}
	if err != nil {
		return Values{}, err
	}
	var v Values
	if err := yaml.Unmarshal(data, &v); err != nil {
		return Values{}, err
	}
	if v.Backends == nil {
		v.Backends = []Backend{}
	}
	return v, nil
}

// Save writes values.yaml, creating its parent directory if needed - e.g.
// config/ on a fresh checkout that's never had a backend added before.
func Save(path string, v Values) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
