package website

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
)

var sample = []Website{
	{Host: "b2b.shop.ddev.site", Code: "b2b", Type: "website"},
	{Host: "fr.shop.ddev.site", Code: "fr", Type: "store"},
}

func TestRenderNginxRun(t *testing.T) {
	got := string(RenderNginxRun(sample))
	want := `map $host $md_run_code {
    default "";
    b2b.shop.ddev.site "b2b";
    fr.shop.ddev.site "fr";
}
map $host $md_run_type {
    default "";
    b2b.shop.ddev.site "website";
    fr.shop.ddev.site "store";
}
`
	if got != want {
		t.Fatalf("got\n%s", got)
	}
}

func TestRenderNginxRunEmpty(t *testing.T) {
	got := string(RenderNginxRun(nil))
	want := "map $host $md_run_code {\n    default \"\";\n}\nmap $host $md_run_type {\n    default \"\";\n}\n"
	if got != want {
		t.Fatalf("got\n%s", got)
	}
}

func TestRenderApacheRun(t *testing.T) {
	got := string(RenderApacheRun(sample))
	want := `SetEnvIfNoCase Host "^b2b[.]shop[.]ddev[.]site(:[0-9]+)?$" MAGE_RUN_CODE=b2b MAGE_RUN_TYPE=website
SetEnvIfNoCase Host "^fr[.]shop[.]ddev[.]site(:[0-9]+)?$" MAGE_RUN_CODE=fr MAGE_RUN_TYPE=store
`
	if got != want {
		t.Fatalf("got\n%s", got)
	}
}

func TestRenderApacheRunEmpty(t *testing.T) {
	if got := RenderApacheRun(nil); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}

func TestBaseURLs(t *testing.T) {
	got := BaseURLs("shop.ddev.site", sample, 9443)
	want := []Setting{
		{"default", "", "web/unsecure/base_url", "https://shop.ddev.site:9443/"},
		{"default", "", "web/secure/base_url", "https://shop.ddev.site:9443/"},
		{"default", "", "web/unsecure/base_link_url", "{{unsecure_base_url}}"},
		{"default", "", "web/secure/base_link_url", "{{secure_base_url}}"},
		{"websites", "b2b", "web/unsecure/base_url", "https://b2b.shop.ddev.site:9443/"},
		{"websites", "b2b", "web/secure/base_url", "https://b2b.shop.ddev.site:9443/"},
		{"websites", "b2b", "web/unsecure/base_link_url", "{{unsecure_base_url}}"},
		{"websites", "b2b", "web/secure/base_link_url", "{{secure_base_url}}"},
		{"stores", "fr", "web/unsecure/base_url", "https://fr.shop.ddev.site:9443/"},
		{"stores", "fr", "web/secure/base_url", "https://fr.shop.ddev.site:9443/"},
		{"stores", "fr", "web/unsecure/base_link_url", "{{unsecure_base_url}}"},
		{"stores", "fr", "web/secure/base_link_url", "{{secure_base_url}}"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v", got)
	}
}

func TestBaseURLsPort443(t *testing.T) {
	got := BaseURLs("shop.ddev.site", nil, 443)
	if len(got) != 4 || got[0].Value != "https://shop.ddev.site/" {
		t.Fatalf("got %v", got)
	}
}

func show(scope, code, path string) string {
	k := "docker exec ddev-shop-web php bin/magento config:show --scope=" + scope
	if code != "" {
		k += " --scope-code=" + code
	}
	return k + " " + path
}

func TestSyncOnlyChanged(t *testing.T) {
	s := site.Site{Name: "shop", Hostnames: []string{"shop.ddev.site", "b2b.shop.ddev.site", "fr.shop.ddev.site"}}
	r := &run.FakeRunner{Out: map[string]string{}, Err: map[string]error{}}
	for _, st := range BaseURLs("shop.ddev.site", sample, 9443) {
		r.Out[show(st.Scope, st.ScopeCode, st.Path)] = st.Value + "\n"
	}
	r.Out[show("websites", "b2b", "web/secure/base_url")] = "https://old.test/\n"
	r.Err[show("websites", "b2b", "web/unsecure/base_url")] = fmt.Errorf("exit 1")
	n, err := Sync(s, sample, r, 9443)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var sets []string
	for _, c := range r.Calls {
		j := strings.Join(c, " ")
		if strings.Contains(j, "config:set") {
			sets = append(sets, j)
		}
	}
	if len(sets) != 2 {
		t.Fatalf("sets %v", sets)
	}
	want := "docker exec ddev-shop-web php bin/magento config:set --lock-env --scope=websites --scope-code=b2b web/secure/base_url https://b2b.shop.ddev.site:9443/"
	if sets[1] != want {
		t.Fatalf("got %q", sets[1])
	}
}

func TestSyncAllMatchedNoSet(t *testing.T) {
	s := site.Site{Name: "shop", Hostnames: []string{"shop.ddev.site"}}
	r := &run.FakeRunner{Out: map[string]string{}}
	for _, st := range BaseURLs("shop.ddev.site", nil, 443) {
		r.Out[show(st.Scope, st.ScopeCode, st.Path)] = st.Value
	}
	n, err := Sync(s, nil, r, 443)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
