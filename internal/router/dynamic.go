package router

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/site"
)

type m = map[string]any

func RenderTraefikSite(s site.Site, crt, key string, ports config.Ports) []byte {
	hosts := make([]string, len(s.Hostnames))
	for i, h := range s.Hostnames {
		hosts[i] = "Host(`" + h + "`)"
	}
	rule := strings.Join(hosts, " || ")
	web := s.Name + "-web"
	mailpit := s.Name + "-mailpit"
	xhgui := s.Name + "-xhgui"
	redirect := s.Name + "-https"
	secure := func(entry, service string) m {
		return m{"rule": rule, "entryPoints": []string{entry}, "service": service, "tls": m{}}
	}
	plain := func(entry, service string) m {
		return m{"rule": rule, "entryPoints": []string{entry}, "service": service}
	}
	svc := func(url string) m {
		return m{"loadBalancer": m{"servers": []m{{"url": url}}}}
	}
	doc := m{
		"http": m{
			"routers": m{
				web + "-secure":     secure("websecure", web),
				web:                 m{"rule": rule, "entryPoints": []string{"web"}, "middlewares": []string{redirect}, "service": web},
				mailpit + "-secure": secure("mailpit-secure", mailpit),
				mailpit:             plain("mailpit", mailpit),
				xhgui + "-secure":   secure("xhgui-secure", xhgui),
				xhgui:               plain("xhgui", xhgui),
			},
			"middlewares": m{
				redirect: m{"redirectScheme": m{"scheme": "https", "port": strconv.Itoa(ports.HTTPS), "permanent": true}},
			},
			"services": m{
				web:     svc(fmt.Sprintf("http://ddev-%s-web:80", s.Name)),
				mailpit: svc(fmt.Sprintf("http://ddev-%s-web:8025", s.Name)),
				xhgui:   svc("http://md-xhgui:80"),
			},
		},
		"tls": m{"certificates": []m{{"certFile": crt, "keyFile": key}}},
	}
	out, _ := yaml.Marshal(doc)
	return out
}

func WriteSite(s site.Site, crt, key, home string, ports config.Ports) error {
	dir := filepath.Join(home, "router", "dynamic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, s.Name+".yaml"), RenderTraefikSite(s, crt, key, ports), 0o644)
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
