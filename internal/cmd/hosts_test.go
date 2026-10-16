package cmd

import (
	"errors"
	"reflect"
	"testing"

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
