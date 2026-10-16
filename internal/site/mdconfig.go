package site

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Website struct {
	Host string `yaml:"host"`
	Code string `yaml:"code"`
	Type string `yaml:"type"`
}

type MDConfig struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	PHP      string `yaml:"php"`
	Database struct {
		Type    string `yaml:"type"`
		Version string `yaml:"version"`
	} `yaml:"database"`
	Search    string    `yaml:"search"`
	Node      string    `yaml:"node"`
	Redis     bool      `yaml:"redis"`
	Hostnames []string  `yaml:"hostnames"`
	Websites  []Website `yaml:"websites"`
	Magento   struct {
		Version string `yaml:"version"`
		Edition string `yaml:"edition"`
	} `yaml:"magento"`
}

var (
	hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	codeRe = regexp.MustCompile(`^[a-z0-9_]+$`)
)

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
	hostnames := []string{c.Name + ".ddev.site"}
	for _, h := range c.Hostnames {
		hostnames = appendUnique(hostnames, h)
	}
	seen := map[string]int{}
	for i, w := range c.Websites {
		if w.Host == "" || w.Code == "" {
			return Site{}, fmt.Errorf("%s: websites[%d] needs host and code", mdConfigPath(root), i)
		}
		host := strings.ToLower(w.Host)
		if !hostRe.MatchString(host) {
			return Site{}, fmt.Errorf("%s: websites[%d] invalid host %q", mdConfigPath(root), i, w.Host)
		}
		if !codeRe.MatchString(w.Code) {
			return Site{}, fmt.Errorf("%s: websites[%d] invalid code %q (use a-z, 0-9, _)", mdConfigPath(root), i, w.Code)
		}
		if j, dup := seen[host]; dup {
			return Site{}, fmt.Errorf("%s: websites[%d] duplicate host %q (same as websites[%d])", mdConfigPath(root), i, w.Host, j)
		}
		seen[host] = i
		c.Websites[i].Host = host
		w.Host = host
		if w.Type == "" {
			c.Websites[i].Type = "website"
		} else if w.Type != "website" && w.Type != "store" {
			return Site{}, fmt.Errorf("%s: websites[%d] type must be website or store, got %q", mdConfigPath(root), i, w.Type)
		}
		hostnames = appendUnique(hostnames, w.Host)
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
		Hostnames:     hostnames,
		Websites:      c.Websites,
		SearchVersion: c.Search,
	}
	if c.Redis {
		s.Addons = []string{"redis"}
	}
	return s, nil
}

func appendUnique(l []string, v string) []string {
	if contains(l, v) {
		return l
	}
	return append(l, v)
}
