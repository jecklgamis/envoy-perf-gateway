package config

import (
	"os"

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

type Values struct {
	Backends []Backend `yaml:"backends"`
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

func Save(path string, v Values) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
