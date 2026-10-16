package install

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/guard"
	"github.com/chuccv/m2-docker/internal/migrate"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

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
	c := site.MDConfig{Name: s.Name, Type: s.Type, PHP: s.PHPVersion, Search: s.SearchVersion, Node: s.NodeVersion}
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

func Migrate(root string, r run.Runner, home string, ports config.Ports, up func(site.Site) error) error {
	root, err := filepath.Abs(root)
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
	if err := guard.CheckNoDDEV(r, s, ports); err != nil {
		return err
	}
	if err := guard.CheckExtraCompose(s); err != nil {
		return err
	}
	if err := guard.CheckDBVersion(r, s); err != nil {
		return err
	}
	if err := site.WriteMDConfig(root, FromDDEV(s)); err != nil {
		return err
	}
	s, err = site.Load(root)
	if err != nil {
		return err
	}
	if err := up(s); err != nil {
		return err
	}
	if s.SearchVersion != "" {
		if err := migrate.EnsureSearchMigrated(home, s, r); err != nil {
			return err
		}
	}
	return lockBaseURL(s, r, baseURL(s.Hostnames[0], ports.HTTPS))
}
