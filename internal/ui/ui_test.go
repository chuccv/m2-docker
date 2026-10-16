package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/chuccv/m2-docker/internal/run"
)

type fakeActions struct {
	mu      sync.Mutex
	calls   []string
	started chan struct{}
	release chan struct{}
}

func (f *fakeActions) rec(s string) error {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
	if f.started != nil {
		f.started <- struct{}{}
		<-f.release
	}
	return nil
}
func (f *fakeActions) Up(n string) error      { return f.rec("up " + n) }
func (f *fakeActions) Stop(n string) error    { return f.rec("stop " + n) }
func (f *fakeActions) Restart(n string) error { return f.rec("restart " + n) }
func (f *fakeActions) Poweroff() error        { return f.rec("poweroff") }

func do(h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "127.0.0.1:9900"
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPostRequiresHeader(t *testing.T) {
	fa := &fakeActions{}
	h := NewServer(t.TempDir(), &run.FakeRunner{}, fa)
	if r := do(h, "POST", "/api/sites/a/up", "", nil); r.Code != 403 {
		t.Fatalf("no header: %d", r.Code)
	}
	if r := do(h, "POST", "/api/sites/a/up", "", map[string]string{"X-MD": "1", "Origin": "https://evil.example"}); r.Code != 403 {
		t.Fatalf("evil origin: %d", r.Code)
	}
	if len(fa.calls) != 0 {
		t.Fatalf("actions called: %v", fa.calls)
	}
	r := do(h, "POST", "/api/sites/a/up", "", map[string]string{"X-MD": "1", "Origin": "http://127.0.0.1:9900"})
	if r.Code != 200 || len(fa.calls) != 1 || fa.calls[0] != "up a" {
		t.Fatalf("ok: %d %v", r.Code, fa.calls)
	}
}

func TestRejectsForeignHost(t *testing.T) {
	h := NewServer(t.TempDir(), &run.FakeRunner{}, &fakeActions{})
	req := httptest.NewRequest("GET", "/api/sites", nil)
	req.Host = "evil.example"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("host: %d", rec.Code)
	}
}

func TestListenRejectsNonLoopback(t *testing.T) {
	for _, a := range []string{"0.0.0.0", "192.168.1.5", "example.com", ""} {
		if err := CheckLoopback(a); err == nil {
			t.Fatalf("%q accepted", a)
		}
	}
	for _, a := range []string{"127.0.0.1", "localhost", "::1"} {
		if err := CheckLoopback(a); err != nil {
			t.Fatalf("%q: %v", a, err)
		}
	}
}

func writeSite(t *testing.T, root, name, cfg string) {
	t.Helper()
	d := filepath.Join(root, name, ".ddev")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSitesView(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	writeSite(t, root, "ee249-qa", "name: ee249-qa\ntype: magento2\nphp_version: \"8.5\"\nwebserver_type: nginx-fpm\ndatabase:\n  type: mariadb\n  version: \"10.6\"\n")
	writeSite(t, root, "site-apache", "name: site-apache\ntype: magento2\nphp_version: \"8.3\"\nwebserver_type: apache-fpm\ndatabase:\n  type: mariadb\n  version: \"10.6\"\n")
	writeSite(t, root, "busy", "name: busy\ntype: magento2\nphp_version: \"8.3\"\nwebserver_type: nginx-fpm\ndatabase:\n  type: mariadb\n  version: \"10.6\"\n")
	os.WriteFile(filepath.Join(home, "config.yaml"), []byte("ports:\n  https: 9443\n"), 0o644)
	f := &run.FakeRunner{Out: map[string]string{"docker ps --format " + psFormat: "|ddev-busy\ntrue|md-ee249-qa\n|md-shared\n"}}
	h := newServer(home, f, &fakeActions{}, []string{root})
	r := do(h, "GET", "/api/sites", "", nil)
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	views := decodeSites(t, r.Body.Bytes())
	qa := views["ee249-qa"]
	if qa.URLs["storefront"] != "https://ee249-qa.ddev.site:9443" || qa.URLs["admin"] != "https://ee249-qa.ddev.site:9443/admin" || qa.URLs["mailpit"] != "https://ee249-qa.ddev.site:8026" || qa.URLs["xhgui"] != "https://ee249-qa.ddev.site:8142" {
		t.Fatalf("urls: %v", qa.URLs)
	}
	if qa.Status != "running" || qa.Blocked != "" || qa.PHP != "8.5" {
		t.Fatalf("qa: %+v", qa)
	}
	ap := views["site-apache"]
	if ap.Blocked != "" || !strings.Contains(ap.Note, "apache") || ap.Engine != "" || ap.Source != "ddev" {
		t.Fatalf("apache: %+v", ap)
	}
	if qa.Engine != "md" {
		t.Fatalf("qa engine: %+v", qa)
	}
	b := views["busy"]
	if b.Status != "ddev" || b.Engine != "ddev" || b.Blocked != "" {
		t.Fatalf("busy: %+v", b)
	}
}

func TestStatsGroupsBySite(t *testing.T) {
	out := `{"Name":"ddev-a-web","CPUPerc":"12.50%","MemUsage":"512MiB / 31GiB"}
{"Name":"ddev-a-db","CPUPerc":"1.00%","MemUsage":"1.5GiB / 31GiB"}
{"Name":"md-router","CPUPerc":"0.20%","MemUsage":"800kB / 31GiB"}
`
	f := &run.FakeRunner{Out: map[string]string{"docker stats --no-stream --format {{json .}}": out}}
	h := NewServer(t.TempDir(), f, &fakeActions{})
	r := do(h, "GET", "/api/stats", "", nil)
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	st := decodeStats(t, r.Body.Bytes())
	if len(st.Sites) != 1 || st.Sites[0].Name != "a" {
		t.Fatalf("sites: %+v", st.Sites)
	}
	a := st.Sites[0]
	if a.CPUPercent != 13.5 || a.MemBytes != 512<<20+3<<29 {
		t.Fatalf("a: %+v", a)
	}
	if st.Shared.MemBytes != 800*1000 || len(st.Containers) != 3 {
		t.Fatalf("shared: %+v %d", st.Shared, len(st.Containers))
	}
}

func TestParseMem(t *testing.T) {
	cases := map[string]int64{"512MiB / 31.2GiB": 512 << 20, "1.2GiB / 3GiB": 1288490188, "800kB / 1GiB": 800000, "0B / 0B": 0, "2MB / 1GB": 2000000}
	for in, want := range cases {
		if got := parseMem(in); got != want {
			t.Errorf("%q: %d want %d", in, got, want)
		}
	}
	if parseCPU("12.50%") != 12.5 {
		t.Error("cpu")
	}
}

func cleanRunner() *run.FakeRunner {
	return &run.FakeRunner{Out: map[string]string{
		"docker volume ls -f dangling=true -q": "foo\nx-mariadb\nmd-keep\n",
	}}
}

func TestCleanNeedsYes(t *testing.T) {
	f := cleanRunner()
	h := NewServer(t.TempDir(), f, &fakeActions{})
	hdr := map[string]string{"X-MD": "1"}
	for _, body := range []string{"", `{"confirm":"no"}`, `{}`} {
		if r := do(h, "POST", "/api/clean", body, hdr); r.Code != 400 {
			t.Fatalf("%q: %d", body, r.Code)
		}
	}
	for _, c := range f.Calls {
		if len(c) > 2 && (c[1] == "volume" && c[2] == "rm" || c[1] == "rmi") {
			t.Fatalf("applied: %v", c)
		}
	}
	if r := do(h, "POST", "/api/clean", `{"confirm":"yes"}`, hdr); r.Code != 200 {
		t.Fatalf("yes: %d %s", r.Code, r.Body)
	}
	var rm []string
	for _, c := range f.Calls {
		if len(c) > 3 && c[1] == "volume" && c[2] == "rm" {
			rm = append(rm, c[3])
		}
	}
	if len(rm) != 1 || rm[0] != "foo" {
		t.Fatalf("removed: %v", rm)
	}
}

func TestOneActionPerSite(t *testing.T) {
	fa := &fakeActions{started: make(chan struct{}), release: make(chan struct{})}
	h := NewServer(t.TempDir(), &run.FakeRunner{}, fa)
	hdr := map[string]string{"X-MD": "1"}
	done := make(chan int)
	go func() { done <- do(h, "POST", "/api/sites/a/up", "", hdr).Code }()
	<-fa.started
	if r := do(h, "POST", "/api/sites/a/up", "", hdr); r.Code != 409 {
		t.Fatalf("second: %d", r.Code)
	}
	close(fa.release)
	if c := <-done; c != 200 {
		t.Fatalf("first: %d", c)
	}
}

func TestDisk(t *testing.T) {
	out := `{"Images":[{"Repository":"md/php","Tag":"8.5","Size":"713MB","Containers":"1"}],"Volumes":[{"Name":"pfsm248-mariadb","Size":"186.8MB"},{"Name":"md-node-24","Size":"146MB"}]}`
	f := &run.FakeRunner{Out: map[string]string{"docker system df -v --format {{json .}}": out}}
	h := NewServer(t.TempDir(), f, &fakeActions{})
	r := do(h, "GET", "/api/disk", "", nil)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"site":"pfsm248"`) || !strings.Contains(r.Body.String(), "md/php") {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
}

func TestDdevActions(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	cfg := "name: %s\ntype: magento2\nphp_version: \"8.3\"\nwebserver_type: apache-fpm\ndatabase:\n  type: mariadb\n  version: \"10.6\"\n"
	writeSite(t, root, "dashboard", fmt.Sprintf(cfg, "dashboard"))
	writeSite(t, root, "x", fmt.Sprintf(cfg, "x"))
	writeSite(t, root, "mdsite", fmt.Sprintf(cfg, "mdsite"))
	os.MkdirAll(filepath.Join(root, "mdsite", ".md"), 0o755)
	os.WriteFile(filepath.Join(root, "mdsite", ".md", "config.yaml"), []byte("name: mdsite\nphp: \"8.3\"\ndatabase:\n  type: mariadb\n  version: \"10.6\"\n"), 0o644)
	f := &run.FakeRunner{Out: map[string]string{"docker ps --format " + psFormat: "|ddev-dashboard\n"}}
	fa := &fakeActions{}
	h := newServer(home, f, fa, []string{root})
	hdr := map[string]string{"X-MD": "1"}
	for _, p := range []string{"/api/sites/dashboard/stop", "/api/sites/dashboard/restart", "/api/sites/x/up", "/api/sites/mdsite/up"} {
		if r := do(h, "POST", p, "", hdr); r.Code != 200 {
			t.Fatalf("%s: %d %s", p, r.Code, r.Body)
		}
	}
	if r := do(h, "POST", "/api/sites/dashboard/up", "", hdr); r.Code == 200 {
		t.Fatal("up on running ddev site accepted")
	}
	var ddev []string
	for _, c := range f.Calls {
		if c[0] == "ddev" {
			ddev = append(ddev, strings.Join(c, " "))
		}
	}
	if strings.Join(ddev, ",") != "ddev stop dashboard,ddev restart dashboard,ddev start x" {
		t.Fatalf("ddev calls: %v", ddev)
	}
	if strings.Join(fa.calls, ",") != "up mdsite" {
		t.Fatalf("md calls: %v", fa.calls)
	}
}

func ddevCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "global_config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDdevSiteLinksUseDdevPorts(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	cfg := "name: %s\ntype: magento2\nphp_version: \"8.3\"\nwebserver_type: nginx-fpm\ndatabase:\n  type: mariadb\n  version: \"10.6\"\n"
	writeSite(t, root, "busy", fmt.Sprintf(cfg, "busy"))
	writeSite(t, root, "idle", fmt.Sprintf(cfg, "idle"))
	writeSite(t, root, "mdstopped", fmt.Sprintf(cfg, "mdstopped"))
	os.MkdirAll(filepath.Join(root, "mdstopped", ".md"), 0o755)
	os.WriteFile(filepath.Join(root, "mdstopped", ".md", "config.yaml"), []byte("name: mdstopped\nphp: \"8.3\"\ndatabase:\n  type: mariadb\n  version: \"10.6\"\n"), 0o644)
	os.WriteFile(filepath.Join(home, "config.yaml"), []byte("ports:\n  https: 9443\n"), 0o644)
	old := ddevGlobalConfig
	defer func() { ddevGlobalConfig = old }()
	f := &run.FakeRunner{Out: map[string]string{"docker ps --format " + psFormat: "|ddev-busy\n"}}
	for _, tc := range []struct{ cfg, base, mail, xh string }{
		{filepath.Join(t.TempDir(), "missing.yaml"), "https://%s.ddev.site", "https://%s.ddev.site:8026", "https://%s.ddev.site:8142"},
		{ddevCfg(t, "router_https_port: \"8443\"\nmailpit_https_port: \"9026\"\nxhgui_https_port: \"9142\"\n"), "https://%s.ddev.site:8443", "https://%s.ddev.site:9026", "https://%s.ddev.site:9142"},
	} {
		ddevGlobalConfig = tc.cfg
		h := newServer(home, f, &fakeActions{}, []string{root})
		views := decodeSites(t, do(h, "GET", "/api/sites", "", nil).Body.Bytes())
		for _, n := range []string{"busy", "idle"} {
			u := views[n].URLs
			if u["storefront"] != fmt.Sprintf(tc.base, n) || u["admin"] != fmt.Sprintf(tc.base, n)+"/admin" || u["mailpit"] != fmt.Sprintf(tc.mail, n) || u["xhgui"] != fmt.Sprintf(tc.xh, n) {
				t.Fatalf("%s: %v", n, u)
			}
		}
		if u := views["mdstopped"].URLs; u["storefront"] != "https://mdstopped.ddev.site:9443" {
			t.Fatalf("md source: %v", u)
		}
	}
}

func TestOriginHostMustMatchRequestHost(t *testing.T) {
	fa := &fakeActions{}
	h := NewServer(t.TempDir(), &run.FakeRunner{}, fa)
	if r := do(h, "POST", "/api/sites/a/up", "", map[string]string{"X-MD": "1", "Origin": "http://127.0.0.1:9901"}); r.Code != 403 {
		t.Fatalf("other loopback port: %d", r.Code)
	}
	if r := do(h, "POST", "/api/sites/a/up", "", map[string]string{"X-MD": "1", "Origin": "http://localhost:9900"}); r.Code != 403 {
		t.Fatalf("other loopback name: %d", r.Code)
	}
	if len(fa.calls) != 0 {
		t.Fatalf("actions called: %v", fa.calls)
	}
}
