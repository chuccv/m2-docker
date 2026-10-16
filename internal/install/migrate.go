package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/guard"
	"github.com/chuccv/m2-docker/internal/migrate"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
	"github.com/chuccv/m2-docker/internal/stack"
)

var stderr io.Writer = os.Stderr

func IsBlank(root string) (bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	for _, e := range entries {
		if e.Name() != ".git" && e.Name() != ".md" {
			return false, nil
		}
	}
	return true, nil
}

func FromDDEV(s site.Site) site.MDConfig {
	c := site.MDConfig{Name: s.Name, Type: s.Type, PHP: s.PHPVersion, Search: s.SearchVersion, SearchEngine: s.SearchEngine, Node: s.NodeVersion, Varnish: site.VarnishSpec(s.Varnish)}
	c.Database.Type = s.DBType
	c.Database.Version = s.DBVersion
	for _, a := range s.Addons {
		if a == "redis" {
			c.Redis = true
		}
	}
	if len(s.Hostnames) > 1 {
		c.Hostnames = append([]string{}, s.Hostnames[1:]...)
	}
	return c
}

func Migrate(root string, r run.Runner, home string, ports config.Ports, up func(site.Site) error) (err error) {
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	if site.HasMDConfig(root) {
		return fmt.Errorf("%s đã migrate: dùng `md up`", root)
	}
	if !exists(filepath.Join(root, ".ddev", "config.yaml")) {
		return fmt.Errorf("%s không có .ddev/config.yaml: không có gì để migrate", root)
	}
	s, err := site.Load(root)
	if err != nil {
		return err
	}
	if err := site.Supported(s); err != nil {
		return err
	}
	if s.HasAddon("mutagen") {
		fmt.Fprintln(stderr, "mutagen bỏ qua trên Linux")
	}
	if err := guard.CheckNoDDEV(r, s, ports); err != nil {
		return err
	}
	if err := guard.CheckExtraCompose(s); err != nil {
		return err
	}
	if err := guard.CheckDBVersion(r, s); err != nil {
		return err
	}
	mdDir := filepath.Join(root, ".md")
	mdDirExisted := exists(mdDir)
	upRan := false
	backedUp := false
	envPath := filepath.Join(root, "app", "etc", "env.php")
	backup := filepath.Join(home, "sites", s.Name, "env.php.pre-migrate")
	defer func() {
		if err == nil {
			return
		}
		os.Remove(filepath.Join(mdDir, "config.yaml"))
		if !mdDirExisted {
			os.Remove(mdDir)
		}
		if upRan && exists(stack.ComposeFile(home, s.Name)) {
			var versions []string
			if key := s.SearchKey(); key != "" {
				versions = []string{key}
			}
			if derr := stack.Down(r, home, s.Name, versions); derr != nil {
				err = fmt.Errorf("%w; compose down: %v", err, derr)
			}
		}
		if backedUp {
			fmt.Fprintf(os.Stderr, "env.php đã bị md sửa (index prefix/base_url). Bản gốc: %s. Khôi phục: cp %s %s\n", backup, backup, envPath)
		}
	}()
	if err := site.WriteMDConfig(root, FromDDEV(s)); err != nil {
		return err
	}
	s, err = site.Load(root)
	if err != nil {
		return err
	}
	warnDDEVNginx(root)
	if data, rerr := os.ReadFile(envPath); rerr == nil {
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(backup, data, 0o600); err != nil {
			return err
		}
		backedUp = true
	}
	upRan = true
	if err := up(s); err != nil {
		return err
	}
	if s.SearchVersion != "" {
		if err := migrate.EnsureSearchMigrated(home, s, r); err != nil {
			return err
		}
	}
	return nil
}

func warnDDEVNginx(root string) {
	for _, g := range []string{"nginx_full", "nginx"} {
		if m, _ := filepath.Glob(filepath.Join(root, ".ddev", g, "*.conf")); len(m) > 0 {
			fmt.Fprintln(os.Stderr, "warning: nginx custom của DDEV không được dùng; khai báo websites trong .md/config.yaml")
			return
		}
	}
}
