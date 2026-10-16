package snapshot

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

const dumpKey = "docker exec ddev-ee249-qa-db mariadb-dump --single-transaction --routines --triggers -udb -pdb db"

func newSite(t *testing.T) site.Site {
	t.Helper()
	return site.Site{Name: "ee249-qa", Root: t.TempDir()}
}

func gunzipFile(t *testing.T, p string) string {
	t.Helper()
	fh, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	zr, err := gzip.NewReader(fh)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	b.ReadFrom(zr)
	return b.String()
}

func TestCreate(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	f := &run.FakeRunner{Out: map[string]string{dumpKey: "SELECT 1;"}}
	p, err := Create(home, s, f, "before")
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(home, "sites", "ee249-qa", "snapshots", "ee249-qa-before.sql.gz") {
		t.Fatalf("path %s", p)
	}
	if gunzipFile(t, p) != "SELECT 1;" {
		t.Fatal("content")
	}
	if len(f.Calls) != 1 || f.Calls[0][2] != "ddev-ee249-qa-db" {
		t.Fatalf("calls %v", f.Calls)
	}
}

func TestCreateTimestampLabel(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	now = func() time.Time { return time.Date(2026, 10, 16, 9, 8, 7, 0, time.UTC) }
	defer func() { now = time.Now }()
	p, err := Create(home, s, &run.FakeRunner{}, "")
	if err != nil || filepath.Base(p) != "ee249-qa-20261016-090807.sql.gz" {
		t.Fatalf("%s %v", p, err)
	}
}

func TestCreateFailureLeavesNothing(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	f := &run.FakeRunner{Err: map[string]error{dumpKey: errors.New("boom")}}
	if _, err := Create(home, s, f, "x"); err == nil {
		t.Fatal("want error")
	}
	entries, _ := os.ReadDir(filepath.Join(home, "sites", "ee249-qa", "snapshots"))
	if len(entries) != 0 {
		t.Fatalf("leftovers %v", entries)
	}
}

func TestRestore(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	f := &run.FakeRunner{Out: map[string]string{dumpKey: "CREATE TABLE t;"}}
	p, _ := Create(home, s, f, "a")
	r := &run.FakeRunner{}
	if err := Restore(home, s, r, p); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"docker", "exec", "ddev-ee249-qa-db", "mariadb-dump", "--single-transaction", "--routines", "--triggers", "-udb", "-pdb", "db"},
		{"docker", "exec", "-i", "ddev-ee249-qa-db", "mariadb", "-udb", "-pdb", "-e", "DROP DATABASE IF EXISTS db; CREATE DATABASE db"},
		{"docker", "exec", "-i", "ddev-ee249-qa-db", "mariadb", "-udb", "-pdb", "db"},
	}
	if !reflect.DeepEqual(r.Calls, want) {
		t.Fatalf("calls %v", r.Calls)
	}
	if string(r.Stdins["docker exec -i ddev-ee249-qa-db mariadb -udb -pdb db"]) != "CREATE TABLE t;" {
		t.Fatal("stdin")
	}
}

func TestRestoreRefusesBadFile(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	r := &run.FakeRunner{}
	if err := Restore(home, s, r, filepath.Join(s.Root, "missing.sql.gz")); err == nil {
		t.Fatal("missing")
	}
	bad := filepath.Join(s.Root, "bad.sql.gz")
	os.WriteFile(bad, []byte("plain sql"), 0o644)
	if err := Restore(home, s, r, bad); err == nil {
		t.Fatal("not gzip")
	}
	if len(r.Calls) != 0 {
		t.Fatalf("ran commands: %v", r.Calls)
	}
}

func TestList(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	dir := filepath.Join(home, "sites", "ee249-qa", "snapshots")
	os.MkdirAll(dir, 0o755)
	for i, n := range []string{"old.sql.gz", "new.sql.gz"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, nil, 0o644)
		ts := time.Now().Add(time.Duration(i) * time.Hour)
		os.Chtimes(p, ts, ts)
	}
	got, err := List(home, s)
	if err != nil || len(got) != 2 || filepath.Base(got[0]) != "new.sql.gz" {
		t.Fatalf("%v %v", got, err)
	}
}

const importKey = "docker exec -i ddev-ee249-qa-db mariadb -udb -pdb db"

func TestRestorePreSnapshotFailureNoDrop(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	p, _ := Create(home, s, &run.FakeRunner{Out: map[string]string{dumpKey: "X"}}, "a")
	r := &run.FakeRunner{Err: map[string]error{dumpKey: errors.New("boom")}}
	if err := Restore(home, s, r, p); err == nil {
		t.Fatal("want error")
	}
	for _, c := range r.Calls {
		if c[len(c)-2] == "-e" {
			t.Fatalf("dropped: %v", c)
		}
	}
}

func TestRestoreImportFailureMentionsPreSnapshot(t *testing.T) {
	s := newSite(t)
	home := t.TempDir()
	p, _ := Create(home, s, &run.FakeRunner{Out: map[string]string{dumpKey: "X"}}, "a")
	now = func() time.Time { return time.Date(2026, 10, 16, 9, 8, 7, 0, time.UTC) }
	defer func() { now = time.Now }()
	r := &run.FakeRunner{Err: map[string]error{importKey: errors.New("boom")}}
	err := Restore(home, s, r, p)
	pre := filepath.Join(home, "sites", "ee249-qa", "snapshots", "ee249-qa-pre-restore-20261016-090807.sql.gz")
	if err == nil || !strings.Contains(err.Error(), pre) {
		t.Fatalf("err %v", err)
	}
	if _, e := os.Stat(pre); e != nil {
		t.Fatal("pre-restore file missing")
	}
}
