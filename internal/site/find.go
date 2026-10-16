package site

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxScanDepth = 3

func Find(nameOrEmpty, cwd string, scanRoots []string) (Site, error) {
	if nameOrEmpty == "" {
		return findByCwd(cwd)
	}
	for _, root := range scanRoots {
		if s, ok := scan(root, nameOrEmpty, 0); ok {
			return s, nil
		}
	}
	return Site{}, fmt.Errorf("site %q not found in %s (depth %d)", nameOrEmpty, strings.Join(scanRoots, ", "), maxScanDepth)
}

func findByCwd(cwd string) (Site, error) {
	dir := cwd
	for {
		if hasConfig(dir) {
			return Load(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Site{}, fmt.Errorf("no .ddev/config.yaml found from %s upward", cwd)
		}
		dir = parent
	}
}

func scan(dir, name string, depth int) (Site, bool) {
	if hasConfig(dir) {
		if s, err := Load(dir); err == nil && s.Name == name {
			return s, true
		}
	}
	if depth >= maxScanDepth {
		return Site{}, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Site{}, false
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if s, ok := scan(filepath.Join(dir, e.Name()), name, depth+1); ok {
			return s, true
		}
	}
	return Site{}, false
}

func hasConfig(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".ddev", "config.yaml"))
	return err == nil
}
