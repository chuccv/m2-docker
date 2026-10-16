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
	marker := filepath.Join(home, "sites", s.Name, ".md-migrated")
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	web := []string{"exec", "-u", "1000", "-w", "/var/www/html", "ddev-" + s.Name + "-web", "php", "bin/magento"}
	steps := []struct {
		label string
		args  []string
	}{
		{"config:set opensearch_index_prefix", []string{"config:set", "--lock-env", "catalog/search/opensearch_index_prefix", Prefix(s.Name)}},
		{"indexer:reindex catalogsearch_fulltext", []string{"indexer:reindex", "catalogsearch_fulltext"}},
	}
	for _, st := range steps {
		args := append(append([]string{}, web...), st.args...)
		if _, err := r.Run("docker", args...); err != nil {
			return fmt.Errorf("search migration step %s: %w", st.label, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return err
	}
	return os.WriteFile(marker, nil, 0o644)
}
