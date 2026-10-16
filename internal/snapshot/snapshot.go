package snapshot

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

var now = time.Now

func dir(home string, s site.Site) string {
	return filepath.Join(home, "sites", s.Name, "snapshots")
}

func Create(home string, s site.Site, r run.Runner, label string) (string, error) {
	if label == "" {
		label = now().Format("20060102-150405")
	}
	if err := os.MkdirAll(dir(home, s), 0o755); err != nil {
		return "", err
	}
	final := filepath.Join(dir(home, s), s.Name+"-"+label+".sql.gz")
	tmp, err := os.CreateTemp(dir(home, s), ".partial-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	zw := gzip.NewWriter(tmp)
	err = r.Pipe(nil, zw, "docker", "exec", "ddev-"+s.Name+"-db", "mariadb-dump", "--single-transaction", "--routines", "--triggers", "-udb", "-pdb", "db")
	if err == nil {
		err = zw.Close()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("snapshot create: %w", err)
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", err
	}
	return final, nil
}

func Restore(home string, s site.Site, r run.Runner, path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	zr, err := gzip.NewReader(fh)
	if err != nil {
		return fmt.Errorf("%s is not a gzip snapshot: %w", path, err)
	}
	defer zr.Close()
	pre, err := Create(home, s, r, "pre-restore-"+now().Format("20060102-150405"))
	if err != nil {
		return fmt.Errorf("pre-restore snapshot failed, nothing dropped: %w", err)
	}
	db := "ddev-" + s.Name + "-db"
	if _, err := r.Run("docker", "exec", "-i", db, "mariadb", "-udb", "-pdb", "-e", "DROP DATABASE IF EXISTS db; CREATE DATABASE db"); err != nil {
		return fmt.Errorf("recreate database (pre-restore snapshot: %s): %w", pre, err)
	}
	if err := r.Pipe(zr, io.Discard, "docker", "exec", "-i", db, "mariadb", "-udb", "-pdb", "db"); err != nil {
		return fmt.Errorf("import snapshot (pre-restore snapshot: %s): %w", pre, err)
	}
	return nil
}

func List(home string, s site.Site) ([]string, error) {
	entries, err := os.ReadDir(dir(home, s))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	type file struct {
		path string
		mod  time.Time
	}
	var fs []file
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".gz" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fs = append(fs, file{filepath.Join(dir(home, s), e.Name()), info.ModTime()})
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].mod.After(fs[j].mod) })
	out := make([]string, len(fs))
	for i, x := range fs {
		out[i] = x.path
	}
	return out, nil
}
