package install

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
	"github.com/chuccv/m2-docker/internal/stack"
)

type InitOptions struct {
	Root, Version                         string
	PHP, DB, DBType, Search, SearchEngine string
	Varnish                               bool
	Up                                    func(site.Site) error
}

func Init(o InitOptions, r run.Runner, home string) (s site.Site, err error) {
	root, err := filepath.Abs(o.Root)
	if err != nil {
		return site.Site{}, err
	}
	if site.HasMDConfig(root) {
		return site.Site{}, fmt.Errorf("%s đã có .md/config.yaml: dùng `md up`", root)
	}
	if exists(filepath.Join(root, ".ddev")) {
		return site.Site{}, fmt.Errorf("%s có folder .ddev: dùng `md migrate`", root)
	}
	blank, err := IsBlank(root)
	if err != nil {
		return site.Site{}, err
	}
	if blank {
		return site.Site{}, fmt.Errorf("folder trắng: %s (dùng `md install` để cài Magento mới)", root)
	}
	version, edition := o.Version, "community"
	if v, e, derr := MagentoPackage(root); derr == nil {
		if version == "" {
			version = v
		}
		edition = e
	} else if version == "" {
		return site.Site{}, fmt.Errorf("không đọc được version Magento từ composer.lock (%v): thêm --version", derr)
	}
	st, err := StackFor(home, version)
	if err != nil {
		return site.Site{}, err
	}
	if err := applyOverrides(&st, o.PHP, o.DB, o.DBType, o.Search, o.SearchEngine); err != nil {
		return site.Site{}, err
	}
	name := site.NormalizeName(filepath.Base(root))
	cfg := site.MDConfig{Name: name, Type: "magento2", PHP: st.PHP, Search: st.Search.Version, SearchEngine: st.Search.Engine, Node: st.Node, Redis: true, RedisVersion: st.Redis}
	if o.Varnish {
		cfg.Varnish = site.VarnishSpec(st.Varnish)
	}
	cfg.Database.Type = st.Database.Type
	cfg.Database.Version = st.Database.Version
	cfg.Magento.Version = version
	cfg.Magento.Edition = edition
	vol := site.Site{Name: name, DBType: cfg.Database.Type}.DBVolume()
	if _, verr := r.Run("docker", "volume", "inspect", vol); verr == nil {
		return site.Site{}, fmt.Errorf("volume DB %s đã tồn tại — có thể là DB của site khác", vol)
	}
	mdDir := filepath.Join(root, ".md")
	mdDirExisted := exists(mdDir)
	upRan, volCreated := false, false
	defer func() {
		if err == nil {
			return
		}
		os.Remove(filepath.Join(mdDir, "config.yaml"))
		if !mdDirExisted {
			os.Remove(mdDir)
		}
		if upRan && exists(stack.ComposeFile(home, name)) {
			var versions []string
			if key := (site.Site{SearchVersion: cfg.Search, SearchEngine: cfg.SearchEngine}).SearchKey(); key != "" {
				versions = []string{key}
			}
			if derr := stack.Down(r, home, name, versions); derr != nil {
				err = fmt.Errorf("%w; compose down: %v", err, derr)
			}
		}
		if volCreated {
			err = fmt.Errorf("%w; volume DB %s được giữ lại (không xoá data); có thể chạy `docker volume rm %s`", err, vol, vol)
		}
	}()
	if err = site.WriteMDConfig(root, cfg); err != nil {
		return site.Site{}, err
	}
	if _, err = r.Run("docker", "volume", "create", "--label", "com.md.db="+st.Database.Type+"_"+st.Database.Version, vol); err != nil {
		return site.Site{}, err
	}
	volCreated = true
	if s, err = site.Load(root); err != nil {
		return site.Site{}, err
	}
	upRan = true
	if err = o.Up(s); err != nil {
		return site.Site{}, err
	}
	return s, nil
}
