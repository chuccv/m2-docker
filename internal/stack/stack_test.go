package stack

import (
	"os"
	"strings"
	"testing"

	"github.com/chuccv/m2-docker/internal/run"
)

func TestDownDisconnectsThenDowns(t *testing.T) {
	f := &run.FakeRunner{Out: map[string]string{}, Err: map[string]error{}}
	f.Err["docker network disconnect md-a_default md-router"] = os.ErrNotExist
	err := Down(f, "/h", "a", []string{"2.19.1", "latest"})
	if err != nil {
		t.Fatal(err)
	}
	var cs []string
	for _, c := range f.Calls {
		cs = append(cs, strings.Join(c, " "))
	}
	want := []string{
		"docker network disconnect md-a_default md-router",
		"docker network disconnect md-a_default md-xhgui",
		"docker network disconnect md-a_default md-opensearch-2.19.1",
		"docker network disconnect md-a_default md-opensearch-latest",
		"docker compose -p md-a -f /h/sites/a/compose.yaml down",
	}
	if strings.Join(cs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%v", cs)
	}
}

func TestDownReturnsComposeError(t *testing.T) {
	f := &run.FakeRunner{Out: map[string]string{}, Err: map[string]error{"docker compose -p md-a -f /h/sites/a/compose.yaml down": os.ErrClosed}}
	if err := Down(f, "/h", "a", nil); err != os.ErrClosed {
		t.Fatal(err)
	}
}

func TestComposeFile(t *testing.T) {
	if ComposeFile("/h", "a") != "/h/sites/a/compose.yaml" {
		t.Fatal(ComposeFile("/h", "a"))
	}
}
