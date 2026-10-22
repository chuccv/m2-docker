package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chuccv/magento2-docker/internal/compose"
	"github.com/chuccv/magento2-docker/internal/config"
	"github.com/chuccv/magento2-docker/internal/migrate"
	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
	"github.com/chuccv/magento2-docker/internal/stack"
	"github.com/chuccv/magento2-docker/internal/website"
)

type Options struct {
	Root, Version, Edition                string
	PHP, DB, DBType, Search, SearchEngine string
	Varnish                               bool
	SampleData                            bool
	AdminUser, AdminPassword, AdminEmail  string
	Up                                    func(site.Site) error
	Out                                   io.Writer
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

func searchArgs(engine, version, prefix string) []string {
	if engine == "elasticsearch" {
		kind := "elasticsearch7"
		if strings.HasPrefix(version, "8") {
			kind = "elasticsearch8"
		}
		return []string{"--search-engine=" + kind, "--elasticsearch-host=elasticsearch", "--elasticsearch-port=9200", "--elasticsearch-index-prefix=" + prefix}
	}
	return []string{"--search-engine=opensearch", "--opensearch-host=opensearch", "--opensearch-port=9200", "--opensearch-index-prefix=" + prefix}
}

func applyOverrides(st *Stack, php, db, dbType, search, engine string) error {
	if engine != "" && engine != st.Search.Engine && search == "" {
		return fmt.Errorf("--search-engine %s needs --search <version>", engine)
	}
	for _, ov := range []struct {
		dst *string
		v   string
	}{{&st.PHP, php}, {&st.Database.Version, db}, {&st.Database.Type, dbType}, {&st.Search.Version, search}, {&st.Search.Engine, engine}} {
		if ov.v != "" {
			*ov.dst = ov.v
		}
	}
	if st.Database.Type != "mariadb" && st.Database.Type != "mysql" {
		return fmt.Errorf("invalid database type %q: use mariadb or mysql", st.Database.Type)
	}
	if st.Search.Engine != "opensearch" && st.Search.Engine != "elasticsearch" {
		return fmt.Errorf("invalid search engine %q: use opensearch or elasticsearch", st.Search.Engine)
	}
	return nil
}

func Install(o Options, r run.Runner, home string, ports config.Ports) (err error) {
	root, err := filepath.Abs(o.Root)
	if err != nil {
		return err
	}
	if site.HasMDConfig(root) {
		return fmt.Errorf("%s already has .md/config.yaml: use `md up`", root)
	}
	if exists(filepath.Join(root, ".ddev")) {
		return fmt.Errorf("%s has a .ddev folder: use `md migrate`", root)
	}
	blank, err := IsBlank(root)
	if err != nil {
		return err
	}
	if !blank {
		return fmt.Errorf("folder is not empty: %s (md install only runs on a blank folder)", root)
	}
	if o.Edition != "community" && o.Edition != "enterprise" {
		return fmt.Errorf("invalid edition %q: use community or enterprise", o.Edition)
	}
	if !validPassword(o.AdminPassword) {
		return fmt.Errorf("admin password must be at least 7 characters with letters and digits")
	}
	st, err := StackFor(home, o.Version)
	if err != nil {
		return err
	}
	if err := applyOverrides(&st, o.PHP, o.DB, o.DBType, o.Search, o.SearchEngine); err != nil {
		return err
	}
	if compose.ComposerAuth() == "" {
		return fmt.Errorf("composer auth.json not found: put Magento keys in $COMPOSER_HOME/auth.json or ~/.composer/auth.json")
	}
	name := site.NormalizeName(filepath.Base(root))
	cfg := site.MDConfig{Name: name, Type: "magento2", PHP: st.PHP, Search: st.Search.Version, SearchEngine: st.Search.Engine, Node: st.Node, Redis: true, RedisVersion: st.Redis}
	if o.Varnish {
		cfg.Varnish = site.VarnishSpec(st.Varnish)
	}
	cfg.Database.Type = st.Database.Type
	cfg.Database.Version = st.Database.Version
	cfg.Magento.Version = o.Version
	cfg.Magento.Edition = o.Edition
	vol := site.Site{Name: name, DBType: cfg.Database.Type}.DBVolume()
	if _, verr := r.Run("docker", "volume", "inspect", vol); verr == nil {
		return fmt.Errorf("DB volume %s already exists — it may belong to another site", vol)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
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
			err = fmt.Errorf("%w; DB volume %s was kept (data not deleted); if the install did not finish you can run `docker volume rm %s`", err, vol, vol)
		}
	}()
	if err := site.WriteMDConfig(root, cfg); err != nil {
		return err
	}
	if err := createDBVolume(r, st, vol); err != nil {
		return err
	}
	volCreated = true
	s, err := site.Load(root)
	if err != nil {
		return err
	}
	upRan = true
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
	url := baseURL(name+"."+site.Domain, ports.HTTPS)
	args := []string{"setup:install",
		"--base-url=" + url, "--base-url-secure=" + url, "--use-secure=1", "--use-secure-admin=1",
		"--db-host=db", "--db-name=db", "--db-user=db", "--db-password=db"}
	args = append(args, searchArgs(st.Search.Engine, st.Search.Version, migrate.Prefix(name))...)
	args = append(args,
		"--session-save=redis", "--session-save-redis-host=redis", "--session-save-redis-db=2",
		"--cache-backend=redis", "--cache-backend-redis-server=redis", "--cache-backend-redis-db=0",
		"--page-cache=redis", "--page-cache-redis-server=redis", "--page-cache-redis-db=1",
		"--backend-frontname=admin", "--admin-user="+o.AdminUser, "--admin-password="+o.AdminPassword, "--admin-email="+o.AdminEmail,
		"--admin-firstname=Admin", "--admin-lastname=User", "--language=en_US", "--currency=USD", "--timezone=UTC", "--use-rewrites=1")
	if err = magento(r, name, args...); err != nil {
		return err
	}
	if _, err := website.Sync(s, s.Websites, r, ports.HTTPS); err != nil {
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

func createDBVolume(r run.Runner, st Stack, vol string) error {
	if _, err := r.Run("docker", "volume", "create", "--label", "com.md.db="+st.Database.Type+"_"+st.Database.Version, vol); err != nil {
		return err
	}
	image := site.Site{DBType: st.Database.Type, DBVersion: st.Database.Version}.DBImage()
	_, err := r.Run("docker", "run", "--rm", "--user", "0", "--entrypoint", "chown", "--volume", vol+":/var/lib/mysql:nocopy", image, "1000:1000", "/var/lib/mysql")
	return err
}
