package install

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Stack struct {
	PHP      string `yaml:"php"`
	Database struct {
		Type    string `yaml:"type"`
		Version string `yaml:"version"`
	} `yaml:"database"`
	Search struct {
		Engine  string `yaml:"engine"`
		Version string `yaml:"version"`
	} `yaml:"search"`
	Redis   string `yaml:"redis"`
	Varnish string `yaml:"varnish"`
	Node    string `yaml:"node"`
}

//go:embed stacks.yaml
var embeddedStacks []byte

var versionRe = regexp.MustCompile(`^(2\.4\.\d+)(-p\d+)?$`)

func loadStacks(home string) (map[string]Stack, error) {
	all := map[string]Stack{}
	if err := yaml.Unmarshal(embeddedStacks, &all); err != nil {
		return nil, fmt.Errorf("embedded stacks.yaml: %w", err)
	}
	path := filepath.Join(home, "stacks.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return all, nil
	}
	if err != nil {
		return nil, err
	}
	user := map[string]Stack{}
	if err := yaml.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for k, v := range user {
		all[k] = v
	}
	return all, nil
}

func StackFor(home, magentoVersion string) (Stack, error) {
	all, err := loadStacks(home)
	if err != nil {
		return Stack{}, err
	}
	if s, ok := all[magentoVersion]; ok {
		return s, nil
	}
	if m := versionRe.FindStringSubmatch(magentoVersion); m != nil {
		if s, ok := all[m[1]]; ok {
			return s, nil
		}
	}
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return Stack{}, fmt.Errorf("unsupported Magento version %q (supported: %s)", magentoVersion, strings.Join(keys, ", "))
}
