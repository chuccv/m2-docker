package cmd

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chuccv/magento2-docker/internal/config"
	"github.com/chuccv/magento2-docker/internal/install"
	"github.com/chuccv/magento2-docker/internal/site"
)

func httpProbe(url string) (int, string, error) {
	c := &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := c.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Location"), nil
}

func (e *env) errw() io.Writer {
	if e.errOut == nil {
		return io.Discard
	}
	return e.errOut
}

func (e *env) lookupHost() func(string) ([]string, error) {
	if e.lookup == nil {
		return net.LookupHost
	}
	return e.lookup
}

func (e *env) probe(url string) (int, string, error) {
	if e.httpGet == nil {
		return httpProbe(url)
	}
	return e.httpGet(url)
}

func (e *env) hostsCommand(s site.Site) string {
	for _, r := range e.scanRoots {
		if strings.HasPrefix(s.Root, strings.TrimRight(r, "/")+"/") {
			return "md hosts " + s.Name + " | sudo tee -a /etc/hosts"
		}
	}
	return "cd " + s.Root + " && md hosts | sudo tee -a /etc/hosts"
}

func (e *env) warnHosts(s site.Site) {
	if len(hostsLines([]site.Site{s}, e.lookupHost())) > 0 {
		fmt.Fprintf(e.errw(), "warning: /etc/hosts is missing lines, the domain is not reachable yet. Run: %s\n", e.hostsCommand(s))
	}
}

func (e *env) checkDomain(s site.Site, httpsPort int) error {
	if lines := hostsLines([]site.Site{s}, e.lookupHost()); len(lines) > 0 {
		if !e.tty() {
			return fmt.Errorf("/etc/hosts is missing lines (containers are still running). Run: %s", e.hostsCommand(s))
		}
		fmt.Fprintln(e.writer(), "Adding to /etc/hosts (sudo will ask for your password):")
		if err := e.runner.Pipe(strings.NewReader(strings.Join(lines, "\n")+"\n"), io.Discard, "sudo", "tee", "-a", "/etc/hosts"); err != nil {
			return fmt.Errorf("sudo tee -a /etc/hosts: %w (containers are still running)", err)
		}
	}
	url := siteURL(s.Hostnames[0], httpsPort) + "/"
	status, loc, err := e.probe(url)
	if err != nil {
		return fmt.Errorf("%s is not reachable: %w (containers are still running)", url, err)
	}
	fmt.Fprintf(e.writer(), "%s → %d %s\n", url, status, loc)
	return nil
}

func (e *env) printInitSteps(s site.Site, httpsPort int) {
	if fileExists(filepath.Join(s.Root, "app", "etc", "env.php")) {
		return
	}
	url := siteURL(s.Hostnames[0], httpsPort) + "/"
	w := e.writer()
	fmt.Fprintln(w, "No app/etc/env.php yet. Next steps:")
	fmt.Fprintln(w, "  1. gunzip -c dump.sql.gz | md mysql")
	fmt.Fprintln(w, "  2. md magento setup:config:set --db-host=db --db-name=db --db-user=db --db-password=db --session-save=redis --session-save-redis-host=redis --session-save-redis-db=2 --cache-backend=redis --cache-backend-redis-server=redis --cache-backend-redis-db=0 --page-cache=redis --page-cache-redis-server=redis --page-cache-redis-db=1")
	fmt.Fprintf(w, "  3. md magento setup:store-config:set --base-url=%s --base-url-secure=%s --use-secure=1 --use-secure-admin=1\n", url, url)
	if s.SearchEngine != "" {
		engine, host := "opensearch", "opensearch"
		if s.SearchEngine == "elasticsearch" {
			engine, host = "elasticsearch7", "elasticsearch"
			if strings.HasPrefix(s.SearchVersion, "8") {
				engine = "elasticsearch8"
			}
		}
		fmt.Fprintf(w, "  4. md magento config:set catalog/search/engine %s && md magento config:set catalog/search/%s_server_hostname %s && md magento config:set catalog/search/%s_server_port 9200\n", engine, engine, host, engine)
	}
	fmt.Fprintln(w, "  5. md magento setup:upgrade")
}

func (e *env) initSite(o install.InitOptions) error {
	home, err := e.home()
	if err != nil {
		return err
	}
	cfg, err := config.Load(home)
	if err != nil {
		return err
	}
	o.Up = func(s site.Site) error { return e.up(s, home) }
	s, err := install.Init(o, e.runner, home)
	if err != nil {
		return err
	}
	e.printInitSteps(s, cfg.Ports.HTTPS)
	return e.checkDomain(s, cfg.Ports.HTTPS)
}

func newInitCmd(e *env) *cobra.Command {
	var o install.InitOptions
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Set up a folder that already has Magento code",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			root, err := e.rootArg(args)
			if err != nil {
				return err
			}
			o.Root = root
			return e.initSite(o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Version, "version", "", "Magento version (default: from composer.lock)")
	f.StringVar(&o.PHP, "php", "", "PHP version (default: from Magento version)")
	f.StringVar(&o.DB, "db", "", "MariaDB version (default: from Magento version)")
	f.StringVar(&o.Search, "search", "", "search engine version (default: from Magento version)")
	f.StringVar(&o.DBType, "db-type", "", "mariadb or mysql (default: from Magento version)")
	f.StringVar(&o.SearchEngine, "search-engine", "", "opensearch or elasticsearch (default: from Magento version)")
	f.BoolVar(&o.Varnish, "varnish", false, "add Varnish")
	return cmd
}
