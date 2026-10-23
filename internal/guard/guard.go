package guard

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chuccv/magento2-docker/internal/config"
	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
)

var ddevPorts = []int{80, 443, 8025, 8026, 8142, 8143, 8036, 8037}

var allowedCompose = map[string]bool{
	"opensearch":        true,
	"opensearch_memory": true,
	"redis":             true,
	"rabbitmq":          true,
	"mutagen":           true,
	"varnish":           true,
	"varnish_extras":    true,
	"phpmyadmin":        true,
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
	vol := s.DBVolume()
	if _, err := r.Run("docker", "volume", "inspect", vol); err != nil {
		return fmt.Errorf("DB volume %s not found", vol)
	}
	want := versionRe.FindString(s.DBVersion)
	label, _ := r.Run("docker", "volume", "inspect", "-f", `{{index .Labels "com.md.db"}}`, vol)
	if v := versionRe.FindString(label); v != "" {
		if v != want {
			return fmt.Errorf("DB volume %s is %s but config wants %s", vol, v, s.DBVersion)
		}
		return nil
	}
	files := []string{s.DBVersionFile()}
	if s.DBType != "mysql" {
		files = append(files, "mysql_upgrade_info")
	}
	found := ""
	for _, file := range files {
		out, _ := r.Run("docker", "run", "--rm", "-u", "1000:1000", "--entrypoint", "cat",
			"-v", vol+":/v:ro", s.DBImage(), "/v/"+file)
		if v := versionRe.FindString(out); v != "" {
			found = v
			break
		}
	}
	if found == "" {
		return fmt.Errorf("DB version in volume %s cannot be determined", vol)
	}
	if found != want {
		return fmt.Errorf("DB volume %s is %s but config wants %s", vol, found, s.DBVersion)
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
	for _, mine := range []int{p.HTTP, p.HTTPS, p.MailpitHTTP, p.MailpitHTTPS, p.XHGuiHTTP, p.XHGuiHTTPS, p.PhpMyAdminHTTP, p.PhpMyAdminHTTPS} {
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
