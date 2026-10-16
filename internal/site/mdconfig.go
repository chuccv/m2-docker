package site

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type MDConfig struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	PHP      string `yaml:"php"`
	Database struct {
		Type    string `yaml:"type"`
		Version string `yaml:"version"`
	} `yaml:"database"`
	Search    string   `yaml:"search"`
	Node      string   `yaml:"node"`
	Redis     bool     `yaml:"redis"`
	Hostnames []string `yaml:"hostnames"`
	Magento   struct {
		Version string `yaml:"version"`
		Edition string `yaml:"edition"`
	} `yaml:"magento"`
}

func NormalizeName(folder string) string {
	return strings.ReplaceAll(strings.ToLower(folder), "_", "-")
}

func mdConfigPath(root string) string {
	return filepath.Join(root, ".md", "config.yaml")
}

func HasMDConfig(root string) bool {
	_, err := os.Stat(mdConfigPath(root))
	return err == nil
}

func WriteMDConfig(root string, c MDConfig) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(mdConfigPath(root)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(mdConfigPath(root), data, 0o644)
}

func loadMD(root string) (Site, error) {
	data, err := os.ReadFile(mdConfigPath(root))
	if err != nil {
		return Site{}, fmt.Errorf("read md config: %w", err)
	}
	var c MDConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Site{}, fmt.Errorf("parse %s: %w", mdConfigPath(root), err)
	}
	if c.Name == "" {
		c.Name = NormalizeName(filepath.Base(root))
	}
	if c.Type == "" {
		c.Type = "magento2"
	}
	if c.Node == "" {
		c.Node = "22"
	}
	s := Site{
		Name:          c.Name,
		Root:          root,
		Type:          c.Type,
		PHPVersion:    c.PHP,
		DBType:        c.Database.Type,
		DBVersion:     c.Database.Version,
		NodeVersion:   c.Node,
		Webserver:     "nginx-fpm",
		Hostnames:     append([]string{c.Name + ".ddev.site"}, c.Hostnames...),
		SearchVersion: c.Search,
	}
	if c.Redis {
		s.Addons = []string{"redis"}
	}
	return s, nil
}
