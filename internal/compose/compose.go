package compose

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

type Paths struct {
	Home         string
	Root         string
	ComposerAuth string
}

type file struct {
	Services map[string]service `yaml:"services"`
	Networks map[string]network `yaml:"networks,omitempty"`
	Volumes  map[string]volume  `yaml:"volumes,omitempty"`
}

type service struct {
	Image         string            `yaml:"image"`
	Entrypoint    []string          `yaml:"entrypoint,omitempty"`
	ContainerName string            `yaml:"container_name"`
	Restart       string            `yaml:"restart,omitempty"`
	User          string            `yaml:"user,omitempty"`
	Command       []string          `yaml:"command,omitempty"`
	DependsOn     []string          `yaml:"depends_on,omitempty"`
	Environment   []string          `yaml:"environment,omitempty"`
	Ports         []string          `yaml:"ports,omitempty"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	ExtraHosts    []string          `yaml:"extra_hosts,omitempty"`
	Networks      []string          `yaml:"networks,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
	Healthcheck   *healthcheck      `yaml:"healthcheck,omitempty"`
}

type healthcheck struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval"`
	Retries     int      `yaml:"retries"`
	StartPeriod string   `yaml:"start_period"`
}

type network struct {
	Name string `yaml:"name,omitempty"`
}

type volume struct {
	External bool   `yaml:"external,omitempty"`
	Name     string `yaml:"name,omitempty"`
}

func marshal(f file) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
