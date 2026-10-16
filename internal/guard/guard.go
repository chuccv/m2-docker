package guard

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

var ddevPorts = []int{80, 443, 8025, 8026, 8142, 8143}

var allowedCompose = map[string]bool{
	"opensearch":        true,
	"opensearch_memory": true,
	"redis":             true,
	"mutagen":           true,
	"varnish":           true,
	"elasticsearch":     true,
	"xhgui-extended":    true,
	"xhgui-ext-route":   true,
	"no-os-dashboards":  true,
	"zz-opensearch-mem": true,
}

var versionRe = regexp.MustCompile(`(\d+\.\d+)`)

func CheckNoDDEV(r run.Runner, s site.Site, ports config.Ports) error {
	out, _ := r.Run("docker", "ps", "--format", "{{.Names}}")
	if hasLine(out, "ddev-router") && usesDDEVPort(ports) {
		return fmt.Errorf("ddev-router is running and md ports collide with DDEV: run `ddev poweroff` first")
	}
	out, _ = r.Run("docker", "ps", "-a", "--filter", "label=com.docker.compose.project=ddev-"+s.Name, "--format", `{{.Names}}|{{.Label "com.md.managed"}}`)
	if leftoverDDEV(out) {
		return fmt.Errorf("leftover DDEV containers for %s: run `ddev stop %s` first", s.Name, s.Name)
	}
	return nil
}

func CheckDBVersion(r run.Runner, s site.Site) error {
	if _, err := r.Run("docker", "volume", "inspect", s.Name+"-mariadb"); err != nil {
		return fmt.Errorf("DB volume %s-mariadb not found", s.Name)
	}
	want := versionRe.FindString(s.DBVersion)
	label, _ := r.Run("docker", "volume", "inspect", "-f", `{{index .Labels "com.md.db"}}`, s.Name+"-mariadb")
	if v := versionRe.FindString(label); v != "" {
		if v != want {
			return fmt.Errorf("DB volume %s-mariadb is %s but config wants %s", s.Name, v, s.DBVersion)
		}
		return nil
	}
	found := ""
	for _, file := range []string{"db_mariadb_version.txt", "mysql_upgrade_info"} {
		out, _ := r.Run("docker", "run", "--rm", "-u", "1000:1000", "--entrypoint", "cat",
			"-v", s.Name+"-mariadb:/v:ro", "mariadb:"+s.DBVersion, "/v/"+file)
		if v := versionRe.FindString(out); v != "" {
			found = v
			break
		}
	}
	if found == "" {
		return fmt.Errorf("DB version in volume %s-mariadb cannot be determined", s.Name)
	}
	if found != want {
		return fmt.Errorf("DB volume %s-mariadb is %s but config wants %s", s.Name, found, s.DBVersion)
	}
	return nil
}

func CheckExtraCompose(s site.Site) error {
	if site.HasMDConfig(s.Root) {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(s.Root, ".ddev", "docker-compose.*.yaml"))
	for _, f := range files {
		base := filepath.Base(f)
		name := strings.TrimSuffix(strings.TrimPrefix(base, "docker-compose."), ".yaml")
		if !allowedCompose[name] {
			return fmt.Errorf("unsupported compose file %s", base)
		}
	}
	return nil
}

func hasLine(out, want string) bool {
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) == want {
			return true
		}
	}
	return false
}

func usesDDEVPort(p config.Ports) bool {
	for _, mine := range []int{p.HTTP, p.HTTPS, p.MailpitHTTP, p.MailpitHTTPS, p.XHGuiHTTP, p.XHGuiHTTPS} {
		for _, d := range ddevPorts {
			if mine == d {
				return true
			}
		}
	}
	return false
}

func leftoverDDEV(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		name, managed, _ := strings.Cut(strings.TrimSpace(line), "|")
		if name != "" && managed == "" {
			return true
		}
	}
	return false
}
