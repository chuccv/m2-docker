package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/internal/site"
)

func TestHostsLines(t *testing.T) {
	s := site.Site{Hostnames: []string{"a.ddev.site", "local.test", "ok.test", "gone.test"}}
	lookup := func(h string) ([]string, error) {
		switch h {
		case "ok.test":
			return []string{"127.0.0.1"}, nil
		case "local.test":
			return []string{"10.0.0.5"}, nil
		}
		return nil, errors.New("no such host")
	}
	got := hostsLines([]site.Site{s}, lookup)
	want := []string{"127.0.0.1 gone.test", "127.0.0.1 local.test"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestHostsAllScansEveryRoot(t *testing.T) {
	e, _, root := testEnv(t, false)
	second := t.TempDir()
	for dir, name := range map[string]string{root: "hoststest-a", second: "hoststest-b"} {
		sd := filepath.Join(dir, name, ".ddev")
		if err := os.MkdirAll(sd, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sd, "config.yaml"), []byte("name: "+name+"\ntype: magento2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e.scanRoots = []string{root, second}
	cmd := &cobra.Command{Use: "md", SilenceUsage: true, SilenceErrors: true}
	register(cmd, e)
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"hosts", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"127.0.0.1 hoststest-a.md.site\n", "127.0.0.1 hoststest-b.md.site\n"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}
