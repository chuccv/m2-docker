package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

func Prefix(name string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToLower(name))
}

func EnsureSearchMigrated(home string, s site.Site, r run.Runner) error {
	marker := markerPath(home, s.Name)
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	web := []string{"exec", "-u", "1000", "-w", "/var/www/html", "ddev-" + s.Name + "-web", "php", "bin/magento"}
	prefixPath := "catalog/search/opensearch_index_prefix"
	if s.SearchEngine == "elasticsearch" {
		prefixPath = "catalog/search/elasticsearch7_index_prefix"
	}
	steps := []struct {
		label string
		args  []string
	}{
		{"config:set index_prefix", []string{"config:set", "--lock-env", prefixPath, Prefix(s.Name)}},
		{"indexer:reindex catalogsearch_fulltext", []string{"indexer:reindex", "catalogsearch_fulltext"}},
	}
	for _, st := range steps {
		args := append(append([]string{}, web...), st.args...)
		if out, err := r.Run("docker", args...); err != nil {
			return fmt.Errorf("search migration step %s: %w\n%s", st.label, err, strings.TrimSpace(out))
		}
	}
	return WriteMarker(home, s.Name)
}

func markerPath(home, name string) string {
	return filepath.Join(home, "sites", name, ".md-migrated")
}

func WriteMarker(home, name string) error {
	marker := markerPath(home, name)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return err
	}
	return os.WriteFile(marker, nil, 0o644)
}
