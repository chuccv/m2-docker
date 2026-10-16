package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chuccv/m2-docker/internal/compose"
	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/migrate"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

type Options struct {
	Root, Version, Edition               string
	PHP, DB, Search                      string
	SampleData                           bool
	AdminUser, AdminPassword, AdminEmail string
	Up                                   func(site.Site) error
	Out                                  io.Writer
}

const webRoot = "/var/www/html"

var (
	hasLetter = regexp.MustCompile(`[A-Za-z]`)
	hasDigit  = regexp.MustCompile(`[0-9]`)
)

func validPassword(p string) bool {
	return len(p) >= 7 && hasLetter.MatchString(p) && hasDigit.MatchString(p)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func baseURL(host string, httpsPort int) string {
	if httpsPort == 443 {
		return "https://" + host + "/"
	}
	return fmt.Sprintf("https://%s:%d/", host, httpsPort)
}

func webArgs(name string, cmd ...string) []string {
	return append([]string{"exec", "-u", "1000", "-w", webRoot, "ddev-" + name + "-web"}, cmd...)
}

func redact(cmd []string) string {
	out := make([]string, len(cmd))
	for i, a := range cmd {
		out[i] = a
		for _, p := range []string{"--admin-password=", "--db-password="} {
			if strings.HasPrefix(a, p) {
				out[i] = p + "***"
			}
		}
	}
	return strings.Join(out, " ")
}

func stream(r run.Runner, name string, cmd ...string) error {
	if err := r.Stream("docker", webArgs(name, cmd...)...); err != nil {
		return fmt.Errorf("%s: %w", redact(cmd), err)
	}
	return nil
}

func magento(r run.Runner, name string, args ...string) error {
	return stream(r, name, append([]string{"php", "bin/magento"}, args...)...)
}

func lockBaseURL(s site.Site, r run.Runner, url string) error {
	for _, path := range []string{"web/unsecure/base_url", "web/secure/base_url"} {
		if err := magento(r, s.Name, "config:set", "--lock-env", path, url); err != nil {
			return err
		}
	}
	return nil
}

func disableTwoFactor(s site.Site, r run.Runner) error {
	out, err := r.Run("docker", webArgs(s.Name, "php", "bin/magento", "module:status", "--enabled")...)
	if err != nil {
		return fmt.Errorf("module:status: %w", err)
	}
	on := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		on[strings.TrimSpace(l)] = true
	}
	var mods []string
	for _, m := range []string{"Magento_AdminAdobeImsTwoFactorAuth", "Magento_TwoFactorAuth"} {
		if on[m] {
			mods = append(mods, m)
		}
	}
	if len(mods) == 0 {
		return nil
	}
	if err := magento(r, s.Name, append([]string{"module:disable"}, mods...)...); err != nil {
		return err
	}
	return magento(r, s.Name, "setup:upgrade")
}

func Install(o Options, r run.Runner, home string, ports config.Ports) error {
	root, err := filepath.Abs(o.Root)
	if err != nil {
		return err
	}
	if exists(filepath.Join(root, ".ddev")) {
		return fmt.Errorf("%s has a .ddev folder: use `md migrate`", root)
	}
	blank, err := IsBlank(root)
	if err != nil {
		return err
	}
	if !blank {
		return fmt.Errorf("folder không trống: %s (md install chỉ chạy trên folder trắng)", root)
	}
	if o.Edition != "community" && o.Edition != "enterprise" {
		return fmt.Errorf("invalid edition %q: use community or enterprise", o.Edition)
	}
	if !validPassword(o.AdminPassword) {
		return fmt.Errorf("admin password must be at least 7 characters with letters and digits")
	}
	st, err := StackFor(o.Version)
	if err != nil {
		return err
	}
	for _, ov := range []struct {
		dst *string
		v   string
	}{{&st.PHP, o.PHP}, {&st.DB, o.DB}, {&st.Search, o.Search}} {
		if ov.v != "" {
			*ov.dst = ov.v
		}
	}
	if compose.ComposerAuth() == "" {
		return fmt.Errorf("composer auth.json not found: put Magento keys in $COMPOSER_HOME/auth.json or ~/.composer/auth.json")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	name := site.NormalizeName(filepath.Base(root))
	cfg := site.MDConfig{Name: name, Type: "magento2", PHP: st.PHP, Search: st.Search, Node: st.Node, Redis: true}
	cfg.Database.Type = "mariadb"
	cfg.Database.Version = st.DB
	cfg.Magento.Version = o.Version
	cfg.Magento.Edition = o.Edition
	if err := site.WriteMDConfig(root, cfg); err != nil {
		return err
	}
	vol := name + "-mariadb"
	if _, err := r.Run("docker", "volume", "inspect", vol); err != nil {
		if _, err := r.Run("docker", "volume", "create", "--label", "com.md.db=mariadb_"+st.DB, vol); err != nil {
			return err
		}
	}
	s, err := site.Load(root)
	if err != nil {
		return err
	}
	if err := o.Up(s); err != nil {
		return err
	}
	if err := stream(r, name, "rm", "-rf", "/tmp/md-install"); err != nil {
		return err
	}
	pkg := "magento/project-" + o.Edition + "-edition=" + o.Version
	if err := stream(r, name, "composer", "create-project", "--repository-url=https://repo.magento.com/", pkg, "/tmp/md-install"); err != nil {
		return err
	}
	if err := stream(r, name, "cp", "-a", "/tmp/md-install/.", webRoot+"/"); err != nil {
		return err
	}
	if err := stream(r, name, "rm", "-rf", "/tmp/md-install"); err != nil {
		return err
	}
	url := baseURL(name+".ddev.site", ports.HTTPS)
	err = magento(r, name, "setup:install",
		"--base-url="+url, "--base-url-secure="+url, "--use-secure=1", "--use-secure-admin=1",
		"--db-host=db", "--db-name=db", "--db-user=db", "--db-password=db",
		"--search-engine=opensearch", "--opensearch-host=opensearch", "--opensearch-port=9200", "--opensearch-index-prefix="+migrate.Prefix(name),
		"--session-save=redis", "--session-save-redis-host=redis", "--session-save-redis-db=2",
		"--cache-backend=redis", "--cache-backend-redis-server=redis", "--cache-backend-redis-db=0",
		"--page-cache=redis", "--page-cache-redis-server=redis", "--page-cache-redis-db=1",
		"--backend-frontname=admin", "--admin-user="+o.AdminUser, "--admin-password="+o.AdminPassword, "--admin-email="+o.AdminEmail,
		"--admin-firstname=Admin", "--admin-lastname=User", "--language=en_US", "--currency=USD", "--timezone=UTC", "--use-rewrites=1")
	if err != nil {
		return err
	}
	if err := lockBaseURL(s, r, url); err != nil {
		return err
	}
	if o.SampleData {
		if err := magento(r, name, "sampledata:deploy"); err != nil {
			return err
		}
		if err := magento(r, name, "setup:upgrade"); err != nil {
			return err
		}
	}
	if err := disableTwoFactor(s, r); err != nil {
		return err
	}
	if err := magento(r, name, "deploy:mode:set", "developer"); err != nil {
		return err
	}
	if err := magento(r, name, "cache:flush"); err != nil {
		return err
	}
	if err := migrate.WriteMarker(home, name); err != nil {
		return err
	}
	out := o.Out
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintf(out, "Storefront: %s\nAdmin: %sadmin\nUser: %s\nPassword: %s\n", url, url, o.AdminUser, o.AdminPassword)
	return nil
}
