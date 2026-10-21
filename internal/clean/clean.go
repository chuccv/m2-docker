package clean

import (
	"strings"

	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
)

type Item struct {
	Kind, Name, Size string
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func split(l string) (string, string) {
	n, sz, _ := strings.Cut(l, "\t")
	return n, sz
}

const xhguiImage = "ddev/ddev-xhgui:v1.25.2"

func isDBVolume(v string) bool {
	return strings.HasSuffix(v, "-mariadb") || strings.HasSuffix(v, "-mysql")
}

func protectedVolume(v string) bool {
	if strings.HasPrefix(v, "md-") || isDBVolume(v) {
		return true
	}
	return strings.HasPrefix(v, "ddev-") && strings.HasSuffix(v, "_redis")
}

func Plan(r run.Runner, sites []site.Site) ([]Item, error) {
	var items []Item

	out, err := r.Run("docker", "images", "-f", "dangling=true", "--format", "{{.ID}}\t{{.Size}}")
	if err != nil {
		return nil, err
	}
	for _, l := range lines(out) {
		n, sz := split(l)
		items = append(items, Item{"image", n, sz})
	}

	used, err := r.Run("docker", "ps", "-a", "--format", "{{.Image}}")
	if err != nil {
		return nil, err
	}
	inUse := map[string]bool{}
	for _, l := range lines(used) {
		inUse[l] = true
	}
	out, err = r.Run("docker", "images", "--format", "{{.Repository}}:{{.Tag}}\t{{.Size}}")
	if err != nil {
		return nil, err
	}
	for _, l := range lines(out) {
		n, sz := split(l)
		if strings.HasPrefix(n, "ddev/") && n != xhguiImage && !inUse[n] {
			items = append(items, Item{"image", n, sz})
		}
	}

	out, err = r.Run("docker", "system", "df", "--format", "{{.Type}}\t{{.Reclaimable}}")
	if err != nil {
		return nil, err
	}
	for _, l := range lines(out) {
		n, sz := split(l)
		if n == "Build Cache" && !strings.HasPrefix(sz, "0B") {
			items = append(items, Item{"buildcache", "build cache", sz})
		}
	}

	known := map[string]bool{}
	for _, s := range sites {
		known[s.DBVolume()] = true
	}
	out, err = r.Run("docker", "volume", "ls", "-f", "dangling=true", "-q")
	if err != nil {
		return nil, err
	}
	var orphans []Item
	for _, v := range lines(out) {
		switch {
		case known[v]:
		case isDBVolume(v):
			orphans = append(orphans, Item{"db-orphan", v, ""})
		case protectedVolume(v):
		default:
			items = append(items, Item{"volume", v, ""})
		}
	}
	return append(items, orphans...), nil
}

func Apply(items []Item, r run.Runner) error {
	for _, it := range items {
		var args []string
		switch it.Kind {
		case "image":
			if it.Name == xhguiImage {
				continue
			}
			args = []string{"rmi", it.Name}
		case "buildcache":
			args = []string{"builder", "prune", "-f"}
		case "volume":
			if protectedVolume(it.Name) {
				continue
			}
			args = []string{"volume", "rm", it.Name}
		default:
			continue
		}
		if _, err := r.Run("docker", args...); err != nil {
			return err
		}
	}
	return nil
}
