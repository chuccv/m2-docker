package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chuccv/magento2-docker/internal/run"
)

func writeSnapshot(t *testing.T, e *env, f *run.FakeRunner, root string) string {
	t.Helper()
	key := "docker exec ddev-ee249-qa-db mariadb-dump --single-transaction --routines --triggers -udb -pdb db"
	f.Out = map[string]string{key: "X"}
	if err := exec(t, e, "snapshot", "--label", "a"); err != nil {
		t.Fatal(err)
	}
	home, _ := e.home()
	p := filepath.Join(home, "sites", "ee249-qa", "snapshots", "ee249-qa-a.sql.gz")
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	f.Calls = nil
	return p
}

func TestSnapshotRestoreConfirm(t *testing.T) {
	e, f, root := testEnv(t, false)
	e.out = &strings.Builder{}
	p := writeSnapshot(t, e, f, root)

	e.in = strings.NewReader("no\n")
	if err := exec(t, e, "snapshot", "restore", p); err == nil {
		t.Fatal("want abort")
	}
	if len(f.Calls) != 0 {
		t.Fatalf("calls %v", f.Calls)
	}

	e.in = strings.NewReader("yes\n")
	if err := exec(t, e, "snapshot", "restore", p); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 3 {
		t.Fatalf("calls %v", f.Calls)
	}

	f.Calls = nil
	if err := exec(t, e, "snapshot", "restore", "--yes", p); err != nil || len(f.Calls) != 3 {
		t.Fatalf("%v %v", err, f.Calls)
	}
}

func TestSnapshotList(t *testing.T) {
	e, f, root := testEnv(t, false)
	out := &strings.Builder{}
	e.out = out
	writeSnapshot(t, e, f, root)
	if err := exec(t, e, "snapshot", "--list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ee249-qa-a.sql.gz") {
		t.Fatalf("out %q", out.String())
	}
}

func TestCleanAbortWithoutYes(t *testing.T) {
	e, f, _ := testEnv(t, false)
	out := &strings.Builder{}
	e.out = out
	e.in = strings.NewReader("\n")
	f.Out = map[string]string{
		"docker images -f dangling=true --format {{.ID}}\t{{.Size}}": "abc\t1MB\n",
		"docker volume ls -f dangling=true -q":                       "site-es-mariadb\n",
	}
	if err := exec(t, e, "clean"); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls {
		if len(c) > 1 && (c[1] == "rmi" || c[1] == "volume" && c[2] == "rm") {
			t.Fatalf("deleted: %v", c)
		}
	}
	if !strings.Contains(out.String(), "docker volume rm site-es-mariadb") {
		t.Fatalf("out %q", out.String())
	}
}

func TestCleanApplyOnYes(t *testing.T) {
	e, f, _ := testEnv(t, false)
	e.out = &strings.Builder{}
	e.in = strings.NewReader("yes\n")
	f.Out = map[string]string{"docker images -f dangling=true --format {{.ID}}\t{{.Size}}": "abc\t1MB\n"}
	if err := exec(t, e, "clean"); err != nil {
		t.Fatal(err)
	}
	last := f.Calls[len(f.Calls)-1]
	if strings.Join(last, " ") != "docker rmi abc" {
		t.Fatalf("calls %v", f.Calls)
	}
}

func TestSnapshotRestoreWarnsOnForeignName(t *testing.T) {
	e, f, root := testEnv(t, false)
	out := &strings.Builder{}
	e.out = out
	p := writeSnapshot(t, e, f, root)
	foreign := filepath.Join(filepath.Dir(p), "other-a.sql.gz")
	if err := os.Rename(p, foreign); err != nil {
		t.Fatal(err)
	}
	e.in = strings.NewReader("yes\n")
	if err := exec(t, e, "snapshot", "restore", foreign); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "WARNING") {
		t.Fatalf("out %q", out.String())
	}
}
