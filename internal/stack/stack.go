package stack

import (
	"path/filepath"

	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
)

func ComposeFile(home, name string) string {
	return filepath.Join(home, "sites", name, "compose.yaml")
}

func Down(r run.Runner, home, name string, searchVersions []string) error {
	net := "md-" + name + "_default"
	targets := []string{"md-router", "md-xhgui"}
	for _, v := range searchVersions {
		targets = append(targets, site.SearchContainer(v))
	}
	for _, c := range targets {
		r.Run("docker", "network", "disconnect", net, c)
	}
	return r.Stream("docker", "compose", "-p", "md-"+name, "-f", ComposeFile(home, name), "down")
}
