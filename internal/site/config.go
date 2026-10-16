package site

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Site struct {
	Name, Root, Type, PHPVersion, DBType, DBVersion, NodeVersion, Webserver string
	Hostnames                                                               []string
	SearchVersion, SearchEngine, Varnish                                    string
	PhpMyAdmin                                                              bool
	Addons                                                                  []string
	Websites                                                                []Website
}

type rawConfig struct {
	Name                string   `yaml:"name"`
	Type                string   `yaml:"type"`
	PHPVersion          string   `yaml:"php_version"`
	Webserver           string   `yaml:"webserver_type"`
	NodeVersion         string   `yaml:"nodejs_version"`
	PerformanceMode     string   `yaml:"performance_mode"`
	AdditionalHostnames []string `yaml:"additional_hostnames"`
	AdditionalFQDNs     []string `yaml:"additional_fqdns"`
	Database            struct {
		Type    string `yaml:"type"`
		Version string `yaml:"version"`
	} `yaml:"database"`
}

func (s Site) IsApache() bool {
	return s.Webserver == "apache-fpm"
}

func Load(root string) (Site, error) {
	if HasMDConfig(root) {
		return loadMD(root)
	}
	ddev := filepath.Join(root, ".ddev")
	data, err := os.ReadFile(filepath.Join(ddev, "config.yaml"))
	if err != nil {
		return Site{}, fmt.Errorf("read ddev config: %w", err)
	}
	var c rawConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Site{}, fmt.Errorf("parse %s: %w", filepath.Join(ddev, "config.yaml"), err)
	}
	if c.Name == "" {
		c.Name = NormalizeName(filepath.Base(root))
	}
	s := Site{
		Name:        c.Name,
		Root:        root,
		Type:        c.Type,
		PHPVersion:  c.PHPVersion,
		DBType:      c.Database.Type,
		DBVersion:   c.Database.Version,
		NodeVersion: c.NodeVersion,
		Webserver:   c.Webserver,
		Hostnames:   []string{c.Name + ".ddev.site"},
	}
	for _, h := range c.AdditionalHostnames {
		s.Hostnames = append(s.Hostnames, h+".ddev.site")
	}
	s.Hostnames = append(s.Hostnames, c.AdditionalFQDNs...)

	files, _ := filepath.Glob(filepath.Join(ddev, "docker-compose.*.yaml"))
	for _, f := range files {
		base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "docker-compose."), ".yaml")
		s.Addons = append(s.Addons, base)
	}
	if c.PerformanceMode == "mutagen" && !contains(s.Addons, "mutagen") {
		s.Addons = append(s.Addons, "mutagen")
	}
	sort.Strings(s.Addons)

	if s.NodeVersion == "" {
		s.NodeVersion = "22"
	}
	s.SearchVersion = searchVersion(ddev)
	if s.SearchVersion == "" {
		if v := composeImageTag(ddev, "elasticsearch", "elasticsearch"); v != "" {
			s.SearchVersion, s.SearchEngine = v, "elasticsearch"
		} else if s.HasAddon("elasticsearch") {
			s.SearchVersion, s.SearchEngine = DefaultElasticsearch, "elasticsearch"
		}
	}
	if s.HasAddon("varnish") {
		s.Varnish = composeImageTag(ddev, "varnish", "varnish")
		if s.Varnish == "" {
			s.Varnish = DefaultVarnish
		}
	}
	s.PhpMyAdmin = s.HasAddon("phpmyadmin")
	return s, nil
}

func searchVersion(ddev string) string {
	if data, err := os.ReadFile(filepath.Join(ddev, ".env.opensearch")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && k == "OPENSEARCH_TAG" {
				return strings.Trim(v, `"'`)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(ddev, "docker-compose.opensearch.yaml")); err == nil {
		return "latest"
	}
	return ""
}

const DefaultElasticsearch = "7.17.14"

func composeImageTag(ddev, addon, repo string) string {
	data, err := os.ReadFile(filepath.Join(ddev, "docker-compose."+addon+".yaml"))
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`(?m)^\s*image:\s*["']?(?:[\w.\-]+(?:/[\w.\-]+)*/)?` + repo + `:([\w.\-]+)`).FindSubmatch(data)
	if m == nil {
		return ""
	}
	return string(m[1])
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}
