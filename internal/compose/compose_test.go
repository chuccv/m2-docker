package compose

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chuccv/magento2-docker/internal/config"
	"github.com/chuccv/magento2-docker/internal/site"
)

var update = flag.Bool("update", false, "update golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatalf("%s mismatch\n--- got ---\n%s", name, got)
	}
}

var paths = Paths{Home: "/home/user/.md", Root: "/home/user/m2-docker"}

func loadSite(t *testing.T, name string) site.Site {
	t.Helper()
	s, err := site.Load(filepath.Join("..", "site", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	s.Root = "/var/www/html/" + name
	return s
}

func TestRenderSiteEE249QA(t *testing.T) {
	out, err := RenderSite(loadSite(t, "ee249-qa"), paths)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "site-ee249-qa.yaml", out)
}

func TestRenderSiteDeterministic(t *testing.T) {
	a, _ := RenderSite(loadSite(t, "ee249-qa"), paths)
	b, _ := RenderSite(loadSite(t, "ee249-qa"), paths)
	if string(a) != string(b) {
		t.Fatal("non deterministic")
	}
}

func TestRenderSiteNoRedis(t *testing.T) {
	s := loadSite(t, "pfsm248")
	s.Addons = nil
	out, err := RenderSite(s, paths)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "site-mariadb-noredis.yaml", out)
}

func TestRenderSharedDefaultPorts(t *testing.T) {
	out, err := RenderShared([]string{"latest", "3.0.0"}, config.Config{Ports: config.Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142, PhpMyAdminHTTP: 8036, PhpMyAdminHTTPS: 8037}}, paths)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "shared-default.yaml", out)
}

func TestRenderSharedCustomPorts(t *testing.T) {
	out, err := RenderShared([]string{"latest", "3.0.0"}, config.Config{Ports: config.Ports{HTTP: 9080, HTTPS: 9443, MailpitHTTP: 9025, MailpitHTTPS: 9026, XHGuiHTTP: 9143, XHGuiHTTPS: 9142, PhpMyAdminHTTP: 9036, PhpMyAdminHTTPS: 9037}}, paths)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "shared-custom.yaml", out)
}

func TestSharedOpensearchDisablesSecurityAndInstallsPlugins(t *testing.T) {
	out, err := RenderShared([]string{"latest"}, config.Config{Ports: config.Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142, PhpMyAdminHTTP: 8036, PhpMyAdminHTTPS: 8037}}, paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DISABLE_INSTALL_DEMO_CONFIG=true", "DISABLE_SECURITY_PLUGIN=true", "opensearch-plugin install --batch", "analysis-icu", "analysis-phonetic", "image: opensearchproject/opensearch:latest", "md-opensearch-latest-plugins:/usr/share/opensearch/plugins", "opensearch-docker-entrypoint.sh"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

func TestSharedXhguiResolvesItsOwnUpstream(t *testing.T) {
	out, err := RenderShared(nil, config.Config{Ports: config.Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142, PhpMyAdminHTTP: 8036, PhpMyAdminHTTPS: 8037}}, paths)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "xhgui:127.0.0.1") {
		t.Fatalf("xhgui upstream not resolvable:\n%s", out)
	}
}

func TestSharedHasNoBuildKey(t *testing.T) {
	out, err := RenderShared([]string{"latest", "3.0.0"}, config.Config{Ports: config.Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142, PhpMyAdminHTTP: 8036, PhpMyAdminHTTPS: 8037}}, paths)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "build:") || strings.Contains(string(out), "dockerfile_inline") {
		t.Fatalf("build key rendered:\n%s", out)
	}
}

func TestRenderSharedBindAllInterfaces(t *testing.T) {
	cfg := config.Config{Ports: config.Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142}, BindAllInterfaces: true}
	out, err := RenderShared(nil, cfg, paths)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "127.0.0.1:") && !strings.Contains(string(out), "xhgui:127.0.0.1") || strings.Contains(string(out), "127.0.0.1:80") {
		t.Fatalf("loopback bind with BindAllInterfaces:\n%s", out)
	}
	if !strings.Contains(string(out), "- 80:80") {
		t.Fatalf("missing all-interface port:\n%s", out)
	}
}

func TestRenderSiteComposerAuth(t *testing.T) {
	p := paths
	p.ComposerAuth = "/h/.composer/auth.json"
	out, err := RenderSite(loadSite(t, "ee249-qa"), p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "/h/.composer/auth.json:/home/www/.composer/auth.json:ro") || !strings.Contains(string(out), "COMPOSER_HOME=/home/www/.composer") {
		t.Fatalf("missing auth mount:\n%s", out)
	}
	plain, _ := RenderSite(loadSite(t, "ee249-qa"), paths)
	if strings.Contains(string(plain), "auth.json") {
		t.Fatal("auth mounted without file")
	}
}

func TestRenderSiteApache(t *testing.T) {
	s := loadSite(t, "ee249-qa")
	s.Webserver = "apache-fpm"
	out, err := RenderSite(s, paths)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "/home/user/.md/sites/ee249-qa/apache-run.conf:/etc/apache2/md-run.conf:ro") || !strings.Contains(string(out), "MD_WEBSERVER=apache") || strings.Contains(string(out), "nginx-run.conf") {
		t.Fatalf("got %s", out)
	}
	plain, _ := RenderSite(loadSite(t, "ee249-qa"), paths)
	if strings.Contains(string(plain), "MD_WEBSERVER") || strings.Contains(string(plain), "apache") {
		t.Fatalf("nginx site: %s", plain)
	}
}

func TestRenderSiteMountsNginxRun(t *testing.T) {
	out, err := RenderSite(loadSite(t, "ee249-qa"), paths)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "/home/user/.md/sites/ee249-qa/nginx-run.conf:/etc/nginx/md-run.conf:ro") {
		t.Fatalf("got %s", out)
	}
}

func TestRenderSiteVarnish(t *testing.T) {
	s := loadSite(t, "ee249-qa")
	s.Varnish = "6.0"
	out, err := RenderSite(s, paths)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "site-varnish.yaml", out)
}

func TestRenderVCL(t *testing.T) {
	golden(t, "default.vcl", RenderVCL())
	if strings.Contains(string(RenderVCL()), "{{") || strings.Contains(string(RenderVCL()), "/*") || strings.Contains(string(RenderVCL()), "#") {
		t.Fatal("placeholder or comment left in VCL")
	}
}

func TestRenderSharedElasticsearch(t *testing.T) {
	out, err := RenderShared([]string{"2.19.1", "elasticsearch:7.17.14"}, config.Config{Ports: config.Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142, PhpMyAdminHTTP: 8036, PhpMyAdminHTTPS: 8037}}, paths)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "shared-elasticsearch.yaml", out)
}

func TestRenderSitePhpMyAdmin(t *testing.T) {
	s := loadSite(t, "ee249-qa")
	s.PhpMyAdmin = true
	out, err := RenderSite(s, paths)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "site-phpmyadmin.yaml", out)
}

func TestSharedHasPhpMyAdminEntrypoints(t *testing.T) {
	out, err := RenderShared(nil, config.Config{Ports: config.Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142, PhpMyAdminHTTP: 9036, PhpMyAdminHTTPS: 9037}}, paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--entrypoints.phpmyadmin.address=:9036", "--entrypoints.phpmyadmin-secure.address=:9037", "127.0.0.1:9036:9036", "127.0.0.1:9037:9037"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestRenderSiteMySQL(t *testing.T) {
	s := loadSite(t, "ee249-qa")
	s.DBType, s.DBVersion = "mysql", "8.0"
	out, err := RenderSite(s, paths)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "site-mysql.yaml", out)
}

func TestRenderSiteRedisVersion(t *testing.T) {
	s := loadSite(t, "pfsm248")
	s.Addons = []string{"redis"}
	def, err := RenderSite(s, paths)
	if err != nil || !strings.Contains(string(def), "image: redis:7\n") {
		t.Fatalf("default: %v\n%s", err, def)
	}
	s.RedisVersion = "7.2"
	out, err := RenderSite(s, paths)
	if err != nil || !strings.Contains(string(out), "image: redis:7.2\n") {
		t.Fatalf("7.2: %v\n%s", err, out)
	}
}

func TestRenderSiteCron(t *testing.T) {
	plain, _ := RenderSite(loadSite(t, "ee249-qa"), paths)
	if strings.Contains(string(plain), "MD_CRON") {
		t.Fatal("cron env must be absent by default")
	}
	s := loadSite(t, "ee249-qa")
	s.Cron = true
	out, err := RenderSite(s, paths)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "MD_CRON=1") {
		t.Fatalf("missing MD_CRON=1\n%s", out)
	}
}

func TestRenderSiteRabbitMQ(t *testing.T) {
	s := loadSite(t, "ee249-qa")
	s.Addons = append(s.Addons, "rabbitmq")
	s.RabbitMQUser, s.RabbitMQPass, s.RabbitMQVhost = "magento", "pa$s", "/"
	out, err := RenderSite(s, paths)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "site-rabbitmq.yaml", out)
}
