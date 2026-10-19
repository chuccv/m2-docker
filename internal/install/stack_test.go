package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUserStacks(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "stacks.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestStackForEmbedded(t *testing.T) {
	cases := map[string][5]string{
		"2.4.9":    {"8.5", "11.4", "3.0.0", "7.2", "7.6"},
		"2.4.8":    {"8.4", "11.4", "2.19.1", "7.2", "7.6"},
		"2.4.7-p3": {"8.3", "10.6", "2.12.0", "7.2", "7.4"},
		"2.4.6":    {"8.2", "10.6", "2.5.0", "7.0", "7.3"},
		"2.4.5":    {"8.1", "10.4", "1.2.4", "6.2", "7.0"},
		"2.4.4":    {"8.1", "10.4", "1.2.4", "6.2", "7.0"},
	}
	for v, w := range cases {
		got, err := StackFor(t.TempDir(), v)
		if err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		have := [5]string{got.PHP, got.Database.Version, got.Search.Version, got.Redis, got.Varnish}
		if have != w || got.Database.Type != "mariadb" || got.Search.Engine != "opensearch" || got.Node != "22" {
			t.Errorf("%s: got %+v want %v", v, got, w)
		}
	}
}

func TestStackForUnsupported(t *testing.T) {
	_, err := StackFor(t.TempDir(), "2.3.7")
	if err == nil || !strings.Contains(err.Error(), "2.4.4") || !strings.Contains(err.Error(), "2.4.9") {
		t.Fatalf("err %v", err)
	}
}

func TestStackForPatchKeyOverridesLine(t *testing.T) {
	home := writeUserStacks(t, "\"2.4.7-p5\":\n  php: \"8.2\"\n")
	got, err := StackFor(home, "2.4.7-p5")
	if err != nil || got.PHP != "8.2" {
		t.Fatalf("%+v %v", got, err)
	}
	other, _ := StackFor(home, "2.4.7-p4")
	if other.PHP != "8.3" {
		t.Fatalf("p4 %+v", other)
	}
}

func TestStackForUserReplacesAndAdds(t *testing.T) {
	home := writeUserStacks(t, "\"2.4.8\":\n  php: \"8.3\"\n  search: {engine: elasticsearch, version: \"8.11.0\"}\n\"2.4.10\":\n  php: \"8.6\"\n")
	got, err := StackFor(home, "2.4.8")
	if err != nil || got.PHP != "8.3" || got.Search.Engine != "elasticsearch" || got.Database.Version != "" {
		t.Fatalf("%+v %v", got, err)
	}
	added, err := StackFor(home, "2.4.10")
	if err != nil || added.PHP != "8.6" {
		t.Fatalf("%+v %v", added, err)
	}
}

func TestStackForMalformedUserFile(t *testing.T) {
	home := writeUserStacks(t, "2.4.8: [unclosed\n")
	if _, err := StackFor(home, "2.4.8"); err == nil || !strings.Contains(err.Error(), "stacks.yaml") {
		t.Fatalf("err %v", err)
	}
}
