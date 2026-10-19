package migrate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

func TestPrefix(t *testing.T) {
	if Prefix("ee249") != "ee249" || Prefix("ee249-qa") != "ee249_qa" || Prefix("Foo.Bar") != "foo_bar" {
		t.Fatal("bad prefix")
	}
}

func newSite(t *testing.T) site.Site {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".ddev"), 0o755); err != nil {
		t.Fatal(err)
	}
	return site.Site{Name: "ee249-qa", Root: root}
}

func marker(home string, s site.Site) string {
	return filepath.Join(home, "sites", s.Name, ".md-migrated")
}

func TestEnsureSearchMigrated(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	f := &run.FakeRunner{}
	if err := EnsureSearchMigrated(home, s, f); err != nil {
		t.Fatal(err)
	}
	pre := []string{"docker", "exec", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web"}
	want := [][]string{
		append(append([]string{}, pre...), "php", "bin/magento", "config:set", "--lock-env", "catalog/search/opensearch_index_prefix", "ee249_qa"),
		append(append([]string{}, pre...), "php", "bin/magento", "indexer:reindex", "catalogsearch_fulltext"),
	}
	if !reflect.DeepEqual(f.Calls, want) {
		t.Fatalf("calls %v", f.Calls)
	}
	if _, err := os.Stat(marker(home, s)); err != nil {
		t.Fatal("no marker")
	}
	f2 := &run.FakeRunner{}
	if err := EnsureSearchMigrated(home, s, f2); err != nil || len(f2.Calls) != 0 {
		t.Fatalf("second run %v %v", f2.Calls, err)
	}
}

func TestEnsureSearchMigratedFailure(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	key := "docker exec -u 1000 -w /var/www/html ddev-ee249-qa-web php bin/magento indexer:reindex catalogsearch_fulltext"
	f := &run.FakeRunner{Err: map[string]error{key: errors.New("boom")}}
	err := EnsureSearchMigrated(home, s, f)
	if err == nil || !strings.Contains(err.Error(), "reindex") {
		t.Fatalf("err %v", err)
	}
	if _, e := os.Stat(marker(home, s)); e == nil {
		t.Fatal("marker written on failure")
	}
}

func TestEnsureSearchMigratedElasticsearchPrefix(t *testing.T) {
	s := newSite(t)
	s.SearchEngine = "elasticsearch"
	f := &run.FakeRunner{}
	if err := EnsureSearchMigrated(t.TempDir(), s, f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.Calls[0], " "), "catalog/search/elasticsearch7_index_prefix ee249_qa") {
		t.Fatalf("%v", f.Calls[0])
	}
}

func TestEnsureSearchMigratedElasticsearch8Prefix(t *testing.T) {
	s := newSite(t)
	s.SearchEngine = "elasticsearch"
	s.SearchVersion = "8.11.4"
	f := &run.FakeRunner{}
	if err := EnsureSearchMigrated(t.TempDir(), s, f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(f.Calls[0], " "), "catalog/search/elasticsearch8_index_prefix ee249_qa") {
		t.Fatalf("%v", f.Calls[0])
	}
}
