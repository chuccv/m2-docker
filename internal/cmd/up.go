package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chuccv/magento2-docker/images"
	"github.com/chuccv/magento2-docker/internal/compose"
	"github.com/chuccv/magento2-docker/internal/config"
	"github.com/chuccv/magento2-docker/internal/guard"
	"github.com/chuccv/magento2-docker/internal/migrate"
	"github.com/chuccv/magento2-docker/internal/router"
	"github.com/chuccv/magento2-docker/internal/site"
	"github.com/chuccv/magento2-docker/internal/tls"
	"github.com/chuccv/magento2-docker/internal/website"
)

func imageHash() (string, error) {
	ctx := images.PHP()
	var paths []string
	err := fs.WalkDir(ctx, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			paths = append(paths, p)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		data, err := fs.ReadFile(ctx, p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractImageContext() (string, error) {
	dir, err := os.MkdirTemp("", "md-php-build-")
	if err != nil {
		return "", err
	}
	ctx := images.PHP()
	err = fs.WalkDir(ctx, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := fs.ReadFile(ctx, p)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info, err := d.Info(); err == nil {
			mode = info.Mode().Perm()
		}
		return os.WriteFile(dst, data, mode)
	})
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func (e *env) ensureImage(phpVersion string) error {
	want, err := imageHash()
	if err != nil {
		return err
	}
	tag := "md/php:" + phpVersion
	out, err := e.runner.Run("docker", "image", "inspect", "-f", `{{index .Config.Labels "com.md.hash"}}`, tag)
	if err == nil && strings.TrimSpace(out) == want {
		return nil
	}
	dir, err := extractImageContext()
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := e.runner.Stream("docker", "build", "--label", "com.md.hash="+want, "--build-arg", "PHP_VERSION="+phpVersion, "-t", tag, dir); err != nil {
		return fmt.Errorf("build %s: %w", tag, err)
	}
	return nil
}

func nodeMajor(v string) string {
	major, _, _ := strings.Cut(v, ".")
	return major
}

func (e *env) ensureNodeVolume(nodeVersion string) error {
	major := nodeMajor(nodeVersion)
	vol := "md-node-" + major
	if _, err := e.runner.Run("docker", "volume", "inspect", vol); err == nil {
		return nil
	}
	if _, err := e.runner.Run("docker", "volume", "create", vol); err != nil {
		return err
	}
	if err := e.runner.Stream("docker", "run", "--rm", "-v", vol+":/out", "node:"+major+"-slim", "cp", "-a", "/usr/local/.", "/out/"); err != nil {
		e.runner.Run("docker", "volume", "rm", vol)
		return err
	}
	return nil
}

func (e *env) ensureRedisVolume(s site.Site) error {
	if !s.HasAddon("redis") {
		return nil
	}
	vol := s.RedisVolume()
	if _, err := e.runner.Run("docker", "volume", "inspect", vol); err == nil {
		return nil
	}
	_, err := e.runner.Run("docker", "volume", "create", vol)
	return err
}

func (e *env) ensureCA(home string) error {
	dst := filepath.Join(home, "ca", "rootCA.pem")
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	out, err := e.runner.Run("mkcert", "-CAROOT")
	if err != nil {
		return fmt.Errorf("mkcert -CAROOT: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(strings.TrimSpace(out), "rootCA.pem"))
	if err != nil {
		return fmt.Errorf("read mkcert root CA: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func writeFile(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func (e *env) connect(args ...string) {
	if _, err := e.runner.Run("docker", append([]string{"network", "connect"}, args...)...); err != nil && !strings.Contains(err.Error(), "already exists") {
		fmt.Fprintf(e.writer(), "warning: docker network connect %s: %v\n", strings.Join(args, " "), err)
	}
}

func (e *env) up(s site.Site, home string) error {
	unlock, err := lockHome(home, e.writer())
	if err != nil {
		return err
	}
	defer unlock()
	cfg, err := config.Load(home)
	if err != nil {
		return err
	}
	if err := site.Supported(s); err != nil {
		return err
	}
	if s.HasAddon("mutagen") {
		fmt.Fprintln(e.writer(), "mutagen ignored on Linux")
	}
	if err := guard.CheckNoDDEV(e.runner, s, cfg.Ports); err != nil {
		return err
	}
	if err := guard.CheckExtraCompose(s); err != nil {
		return err
	}
	if err := guard.CheckDBVersion(e.runner, s); err != nil {
		return err
	}
	if err := e.ensureImage(s.PHPVersion); err != nil {
		return err
	}
	if err := e.ensureNodeVolume(s.NodeVersion); err != nil {
		return err
	}
	if err := e.ensureRedisVolume(s); err != nil {
		return err
	}
	if err := e.ensureCA(home); err != nil {
		return err
	}
	st, err := loadState(home)
	if err != nil {
		return err
	}
	st.add(siteRef{Name: s.Name, Root: s.Root}, s.SearchKey())
	st.SearchVersions = searchKeys(st.sites(s), st.SearchVersions)
	paths := compose.Paths{Home: home, Root: s.Root, ComposerAuth: compose.ComposerAuth()}
	for _, d := range []string{"certs", "router/dynamic"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			return err
		}
	}
	shared, err := compose.RenderShared(st.SearchVersions, cfg, paths)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(home, "shared", "xhgui-config.php"), []byte(compose.XHGuiConfig)); err != nil {
		return err
	}
	sharedFile := filepath.Join(home, "shared", "compose.yaml")
	if err := writeFile(sharedFile, shared); err != nil {
		return err
	}
	services := []string{"compose", "-p", "md-shared", "-f", sharedFile, "up", "-d", "--wait", "md-router", "md-xhgui"}
	if key := s.SearchKey(); key != "" {
		services = append(services, site.SearchContainer(key))
	}
	if err := e.runner.Stream("docker", services...); err != nil {
		return err
	}
	dcrt, dkey, err := tls.EnsureDefaultCert(home, e.runner)
	if err != nil {
		return err
	}
	if err := router.WriteDefaultCert("/certs/"+filepath.Base(dcrt), "/certs/"+filepath.Base(dkey), home); err != nil {
		return err
	}
	crt, key, err := tls.EnsureCert(s, home, e.runner)
	if err != nil {
		return err
	}
	if err := router.WriteSite(s, "/certs/"+filepath.Base(crt), "/certs/"+filepath.Base(key), home); err != nil {
		return err
	}
	body, err := compose.RenderSite(s, paths)
	if err != nil {
		return err
	}
	file := siteFile(home, s.Name)
	if err := writeFile(file, body); err != nil {
		return err
	}
	if s.Varnish != "" {
		if err := writeFile(filepath.Join(filepath.Dir(file), "default.vcl"), compose.RenderVCL()); err != nil {
			return err
		}
	}
	runName, runBody, reload := "nginx-run.conf", website.RenderNginxRun(s.Websites), []string{"nginx", "-s", "reload"}
	if s.IsApache() {
		runName, runBody, reload = "apache-run.conf", website.RenderApacheRun(s.Websites), []string{"apachectl", "-k", "graceful"}
	}
	runConf := filepath.Join(filepath.Dir(file), runName)
	prev, perr := os.ReadFile(runConf)
	runChanged := perr == nil && !bytes.Equal(prev, runBody)
	if err := writeFile(runConf, runBody); err != nil {
		return err
	}
	if err := e.runner.Stream("docker", "compose", "-p", "md-"+s.Name, "-f", file, "up", "-d", "--wait"); err != nil {
		return err
	}
	if err := saveState(home, st); err != nil {
		return err
	}
	if _, err := e.runner.Run("docker", "image", "prune", "-f", "--filter", "label=com.md.image=php"); err != nil {
		fmt.Fprintf(e.writer(), "warning: prune old md/php images: %v\n", err)
	}
	if runChanged {
		if _, err := e.runner.Run("docker", append([]string{"exec", "ddev-" + s.Name + "-web"}, reload...)...); err != nil {
			fmt.Fprintf(e.writer(), "warning: %s reload: %v\n", reload[0], err)
		}
	}
	e.connectShared(s)
	if running, err := e.runningProjects(); err == nil {
		for _, ref := range st.Sites {
			if ref.Name == s.Name || !running["md-"+ref.Name] {
				continue
			}
			if o, err := site.Load(ref.Root); err == nil {
				e.connectShared(o)
			}
		}
	}
	if err := e.syncWebsites(s, cfg.Ports.HTTPS); err != nil {
		fmt.Fprintf(e.writer(), "warning: websites sync: %v\n", err)
	}
	e.warnForeignRedirects(s, cfg.Ports.HTTPS)
	for _, h := range s.Hostnames {
		fmt.Fprintln(e.writer(), siteURL(h, cfg.Ports.HTTPS))
	}
	return nil
}

func (e *env) connectShared(s site.Site) {
	net := "md-" + s.Name + "_default"
	if key := s.SearchKey(); key != "" {
		e.connect("--alias", site.SearchAlias(key), net, site.SearchContainer(key))
	}
	e.connect("--alias", "xhgui", net, "md-xhgui")
	var aliases []string
	for _, h := range s.Hostnames {
		aliases = append(aliases, "--alias", h)
	}
	e.connect(append(aliases, net, "md-router")...)
}

func (e *env) syncWebsites(s site.Site, httpsPort int) error {
	if _, err := os.Stat(filepath.Join(s.Root, "app", "etc", "env.php")); err != nil {
		return nil
	}
	changed, err := website.Sync(s, s.Websites, e.runner, httpsPort)
	if err != nil || changed == 0 {
		return err
	}
	_, err = e.runner.Run("docker", "exec", "ddev-"+s.Name+"-web", "php", "bin/magento", "cache:flush")
	return err
}

func (e *env) warnForeignRedirects(s site.Site, httpsPort int) {
	if _, err := os.Stat(filepath.Join(s.Root, "app", "etc", "env.php")); err != nil {
		return
	}
	hosts := []string{s.Hostnames[0]}
	for _, w := range s.Websites {
		hosts = append(hosts, w.Host)
	}
	for _, h := range hosts {
		url := siteURL(h, httpsPort) + "/"
		_, loc, err := e.probe(url)
		if err != nil {
			fmt.Fprintf(e.writer(), "warning: %s: %v\n", url, err)
			continue
		}
		u, err := neturl.Parse(loc)
		if err != nil || u.Host == "" || u.Hostname() == h {
			continue
		}
		fmt.Fprintf(e.writer(), "warning: %s redirects to %s. Check core_config_data web/%%_url for that scope.\n", url, loc)
	}
}

func siteURL(host string, httpsPort int) string {
	if httpsPort == 443 {
		return "https://" + host
	}
	return fmt.Sprintf("https://%s:%d", host, httpsPort)
}

func newUpCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "up [name]",
		Short: "Start a site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return e.upByName(nameArg(args))
		},
	}
}

func nameArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func (e *env) upByName(name string) error {
	s, err := e.findSite(name)
	if err != nil {
		return err
	}
	if !site.HasMDConfig(s.Root) {
		return fmt.Errorf("%s has no .md/config.yaml: run `md migrate` first", s.Root)
	}
	home, err := e.home()
	if err != nil {
		return err
	}
	if err := e.up(s, home); err != nil {
		return err
	}
	e.warnHosts(s)
	if s.SearchVersion == "" || !fileExists(filepath.Join(s.Root, "app", "etc", "env.php")) {
		return nil
	}
	return migrate.EnsureSearchMigrated(home, s, e.runner)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
