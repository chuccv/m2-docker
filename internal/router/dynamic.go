package router

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chuccv/magento2-docker/internal/site"
)

type m = map[string]any

func RenderTraefikSite(s site.Site, crt, key string) []byte {
	hosts := make([]string, len(s.Hostnames))
	for i, h := range s.Hostnames {
		hosts[i] = "Host(`" + h + "`)"
	}
	rule := strings.Join(hosts, " || ")
	web := s.Name + "-web"
	mailpit := s.Name + "-mailpit"
	xhgui := s.Name + "-xhgui"
	xhguiSite := xhgui + "-site"
	secure := func(entry, service string) m {
		return m{"rule": rule, "entryPoints": []string{entry}, "service": service, "tls": m{}}
	}
	plain := func(entry, service string) m {
		return m{"rule": rule, "entryPoints": []string{entry}, "service": service}
	}
	svc := func(url string) m {
		return m{"loadBalancer": m{"servers": []m{{"url": url}}}}
	}
	front := fmt.Sprintf("http://ddev-%s-web:80", s.Name)
	if s.Varnish != "" {
		front = fmt.Sprintf("http://ddev-%s-varnish:80", s.Name)
	}
	doc := m{
		"http": m{
			"routers": m{
				web + "-secure":     secure("websecure", web),
				web:                 plain("web", web),
				mailpit + "-secure": secure("mailpit-secure", mailpit),
				mailpit:             plain("mailpit", mailpit),
				xhgui + "-secure":   withMiddleware(secure("xhgui-secure", xhgui), xhguiSite),
				xhgui:               withMiddleware(plain("xhgui", xhgui), xhguiSite),
			},
			"middlewares": m{
				xhguiSite: m{"headers": m{"customRequestHeaders": m{"X-Md-Site": s.Name + "."}}},
			},
			"services": m{
				web:     svc(front),
				mailpit: svc(fmt.Sprintf("http://ddev-%s-web:8025", s.Name)),
				xhgui:   svc("http://md-xhgui:80"),
			},
		},
		"tls": m{"certificates": []m{{"certFile": crt, "keyFile": key}}},
	}
	if s.PhpMyAdmin {
		pma := s.Name + "-phpmyadmin"
		h := doc["http"].(m)
		r := h["routers"].(m)
		r[pma+"-secure"] = secure("phpmyadmin-secure", pma)
		r[pma] = plain("phpmyadmin", pma)
		h["services"].(m)[pma] = svc(fmt.Sprintf("http://ddev-%s-phpmyadmin:80", s.Name))
	}
	out, _ := yaml.Marshal(doc)
	return out
}

func withMiddleware(r m, middleware string) m {
	r["middlewares"] = []string{middleware}
	return r
}

func WriteSite(s site.Site, crt, key, home string) error {
	dir := filepath.Join(home, "router", "dynamic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.Name+".yaml"), RenderTraefikSite(s, crt, key), 0o644)
}

func RenderDefaultCert(crt, key string) []byte {
	doc := m{"tls": m{"stores": m{"default": m{"defaultCertificate": m{"certFile": crt, "keyFile": key}}}}}
	out, _ := yaml.Marshal(doc)
	return out
}

func WriteDefaultCert(crt, key, home string) error {
	dir := filepath.Join(home, "router", "dynamic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "_default.yaml"), RenderDefaultCert(crt, key), 0o644)
}
