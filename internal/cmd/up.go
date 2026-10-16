package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/images"
	"github.com/chuccv/m2-docker/internal/compose"
	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/guard"
	"github.com/chuccv/m2-docker/internal/migrate"
	"github.com/chuccv/m2-docker/internal/router"
	"github.com/chuccv/m2-docker/internal/site"
	"github.com/chuccv/m2-docker/internal/tls"
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
	_, err = e.runner.Run("docker", "image", "prune", "-f", "--filter", "label=com.md.image=php")
	return err
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
	return e.runner.Stream("docker", "run", "--rm", "-v", vol+":/out", "node:"+major+"-slim", "cp", "-a", "/usr/local/.", "/out/")
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

func (e *env) up(s site.Site, home string, skipMigrate bool) error {
	cfg, err := config.Load(home)
	if err != nil {
		return err
	}
	if err := site.Supported(s); err != nil {
		return err
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
	if err := e.ensureCA(home); err != nil {
		return err
	}
	st, err := loadState(home)
	if err != nil {
		return err
	}
	st.add(siteRef{Name: s.Name, Root: s.Root}, s.SearchVersion)
	if err := saveState(home, st); err != nil {
		return err
	}
	paths := compose.Paths{Home: home, Root: s.Root}
	for _, d := range []string{"certs", "router/dynamic"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			return err
		}
	}
	shared, err := compose.RenderShared(st.SearchVersions, cfg.Ports, paths)
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
	if err := e.runner.Stream("docker", "compose", "-p", "md-shared", "-f", sharedFile, "up", "-d"); err != nil {
		return err
	}
	crt, key, err := tls.EnsureCert(s, home, e.runner)
	if err != nil {
		return err
	}
	if err := router.WriteSite(s, "/certs/"+filepath.Base(crt), "/certs/"+filepath.Base(key), home, cfg.Ports); err != nil {
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
	if err := e.runner.Stream("docker", "compose", "-p", "md-"+s.Name, "-f", file, "up", "-d", "--wait"); err != nil {
		return err
	}
	net := "md-" + s.Name + "_default"
	if s.SearchVersion != "" {
		e.connect("--alias", "opensearch", net, "md-opensearch-"+s.SearchVersion)
	}
	e.connect("--alias", "xhgui", net, "md-xhgui")
	var aliases []string
	for _, h := range s.Hostnames {
		aliases = append(aliases, "--alias", h)
	}
	e.connect(append(aliases, net, "md-router")...)
	if s.SearchVersion != "" && !skipMigrate {
		if err := migrate.EnsureSearchMigrated(s, e.runner); err != nil {
			return err
		}
	}
	for _, h := range s.Hostnames {
		fmt.Fprintln(e.writer(), siteURL(h, cfg.Ports.HTTPS))
	}
	return nil
}

func siteURL(host string, httpsPort int) string {
	if httpsPort == 443 {
		return "https://" + host
	}
	return fmt.Sprintf("https://%s:%d", host, httpsPort)
}

func newUpCmd(e *env) *cobra.Command {
	var skipMigrate bool
	cmd := &cobra.Command{
		Use:   "up [name]",
		Short: "Start a site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return e.upByName(nameArg(args), skipMigrate)
		},
	}
	cmd.Flags().BoolVar(&skipMigrate, "skip-migrate", false, "skip search engine migration")
	return cmd
}

func nameArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func (e *env) upByName(name string, skipMigrate bool) error {
	s, err := e.findSite(name)
	if err != nil {
		return err
	}
	home, err := e.home()
	if err != nil {
		return err
	}
	return e.up(s, home, skipMigrate)
}
