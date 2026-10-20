package website

import (
	"fmt"
	"strings"

	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

type Website = site.Website

type Setting struct{ Scope, ScopeCode, Path, Value string }

func RenderNginxRun(ws []Website) []byte {
	var b strings.Builder
	for _, m := range []struct {
		name string
		val  func(Website) string
	}{
		{"md_run_code", func(w Website) string { return w.Code }},
		{"md_run_type", func(w Website) string { return w.Type }},
	} {
		fmt.Fprintf(&b, "map $host $%s {\n    default \"\";\n", m.name)
		for _, w := range ws {
			fmt.Fprintf(&b, "    %s %q;\n", w.Host, m.val(w))
		}
		b.WriteString("}\n")
	}
	return []byte(b.String())
}

func RenderApacheRun(ws []Website) []byte {
	var b strings.Builder
	for _, w := range ws {
		fmt.Fprintf(&b, "SetEnvIfNoCase Host \"^%s(:[0-9]+)?$\" MAGE_RUN_CODE=%s MAGE_RUN_TYPE=%s\n", strings.ReplaceAll(w.Host, ".", "[.]"), w.Code, w.Type)
	}
	return []byte(b.String())
}

func baseURL(host string, httpsPort int) string {
	if httpsPort == 443 {
		return "https://" + host + "/"
	}
	return fmt.Sprintf("https://%s:%d/", host, httpsPort)
}

func BaseURLs(primary string, ws []Website, httpsPort int) []Setting {
	var out []Setting
	add := func(scope, code, host string) {
		u := baseURL(host, httpsPort)
		out = append(out,
			Setting{scope, code, "web/unsecure/base_url", u},
			Setting{scope, code, "web/secure/base_url", u},
			Setting{scope, code, "web/unsecure/base_link_url", "{{unsecure_base_url}}"},
			Setting{scope, code, "web/secure/base_link_url", "{{secure_base_url}}"})
	}
	add("default", "", primary)
	for _, w := range ws {
		scope := "websites"
		if w.Type == "store" {
			scope = "stores"
		}
		add(scope, w.Code, w.Host)
	}
	return out
}

func scopeArgs(st Setting) []string {
	args := []string{"--scope=" + st.Scope}
	if st.ScopeCode != "" {
		args = append(args, "--scope-code="+st.ScopeCode)
	}
	return args
}

func Sync(s site.Site, ws []Website, r run.Runner, httpsPort int) (int, error) {
	container := "ddev-" + s.Name + "-web"
	changed := 0
	for _, st := range BaseURLs(s.Hostnames[0], ws, httpsPort) {
		args := append([]string{"exec", container, "php", "bin/magento", "config:show"}, scopeArgs(st)...)
		cur, err := r.Run("docker", append(args, st.Path)...)
		if err == nil && strings.TrimSpace(cur) == st.Value {
			continue
		}
		set := append([]string{"exec", container, "php", "bin/magento", "config:set", "--lock-env"}, scopeArgs(st)...)
		if _, err := r.Run("docker", append(set, st.Path, st.Value)...); err != nil {
			return changed, fmt.Errorf("config:set %s %s/%s: %w", st.Path, st.Scope, st.ScopeCode, err)
		}
		changed++
	}
	return changed, nil
}
