package compose

import (
	"fmt"
	"strings"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/site"
)

const XHGuiConfig = `<?php
return [
    'save.handler' => 'pdo',
    'pdo' => [
        'dsn' => 'sqlite:/data/xhgui.sqlite',
        'user' => null,
        'pass' => null,
        'table' => 'results',
        'tableWatch' => 'watches',
        'initSchema' => 'true',
    ],
    'run.view.filter.names' => ['Zend*', 'Composer*'],
    'upload.token' => '',
    'path.prefix' => null,
    'timezone' => getenv('TZ'),
    'date.format' => 'M jS H:i:s',
    'detail.count' => 6,
    'page.limit' => 25,
];
`

func RenderShared(searchVersions []string, cfg config.Config, p Paths) ([]byte, error) {
	ports := cfg.Ports
	bind := "127.0.0.1:"
	if cfg.BindAllInterfaces {
		bind = ""
	}
	entry := []struct {
		name string
		port int
	}{
		{"web", ports.HTTP},
		{"websecure", ports.HTTPS},
		{"mailpit", ports.MailpitHTTP},
		{"mailpit-secure", ports.MailpitHTTPS},
		{"xhgui", ports.XHGuiHTTP},
		{"xhgui-secure", ports.XHGuiHTTPS},
		{"phpmyadmin", ports.PhpMyAdminHTTP},
		{"phpmyadmin-secure", ports.PhpMyAdminHTTPS},
	}
	cmd := []string{
		"--providers.file.directory=/etc/traefik/dynamic",
		"--providers.file.watch=true",
	}
	var portMaps []string
	for _, e := range entry {
		cmd = append(cmd, fmt.Sprintf("--entrypoints.%s.address=:%d", e.name, e.port))
		portMaps = append(portMaps, fmt.Sprintf("%s%d:%d", bind, e.port, e.port))
	}

	f := file{
		Services: map[string]service{
			"md-router": {
				Image:         "traefik:v3.3",
				ContainerName: "md-router",
				Restart:       "no",
				Command:       cmd,
				Ports:         portMaps,
				Volumes:       []string{p.Home + "/router/dynamic:/etc/traefik/dynamic", p.Home + "/certs:/certs:ro"},
				Networks:      []string{"md"},
			},
			"md-xhgui": {
				Image:         "ddev/ddev-xhgui:v1.25.2",
				ContainerName: "md-xhgui",
				Restart:       "no",
				Command:       []string{"sh", "-c", "chown www-data /data && nginx -g 'pid /dev/shm/nginx.pid;' && exec php-fpm"},
				ExtraHosts:    []string{"xhgui:127.0.0.1"},
				Volumes:       []string{"md-xhgui-data:/data", p.Home + "/shared/xhgui-config.php:/var/www/xhgui/config/config.php:ro"},
				Networks:      []string{"md"},
			},
		},
		Networks: map[string]network{"md": {Name: "md"}},
		Volumes:  map[string]volume{"md-xhgui-data": {}},
	}
	health := &healthcheck{Test: []string{"CMD-SHELL", "curl -fs 'localhost:9200/_cluster/health?wait_for_status=yellow&timeout=1s' || exit 1"}, Interval: "5s", Retries: 60, StartPeriod: "10s"}
	for _, key := range searchVersions {
		name := site.SearchContainer(key)
		if v, ok := strings.CutPrefix(key, "elasticsearch:"); ok {
			f.Services[name] = service{
				Image:         "elasticsearch:" + v,
				Entrypoint:    []string{"sh", "-c", "for p in analysis-icu analysis-phonetic; do ./bin/elasticsearch-plugin list | grep -qx $$p || ./bin/elasticsearch-plugin install --batch $$p; done; exec /bin/tini -- /usr/local/bin/docker-entrypoint.sh eswrapper"},
				ContainerName: name,
				Restart:       "no",
				Environment: []string{
					"discovery.type=single-node",
					"xpack.security.enabled=false",
					"ES_JAVA_OPTS=-Xms512m -Xmx512m",
				},
				Volumes:     []string{name + "-data:/usr/share/elasticsearch/data", name + "-plugins:/usr/share/elasticsearch/plugins"},
				Networks:    []string{"md"},
				Healthcheck: health,
			}
		} else {
			f.Services[name] = service{
				Image:         "opensearchproject/opensearch:" + key,
				Entrypoint:    []string{"sh", "-c", "for p in analysis-icu analysis-phonetic; do ./bin/opensearch-plugin list | grep -qx $$p || ./bin/opensearch-plugin install --batch $$p; done; exec ./opensearch-docker-entrypoint.sh opensearch"},
				ContainerName: name,
				Restart:       "no",
				Environment: []string{
					"discovery.type=single-node",
					"DISABLE_INSTALL_DEMO_CONFIG=true",
					"DISABLE_SECURITY_PLUGIN=true",
					"OPENSEARCH_JAVA_OPTS=-Xms768m -Xmx768m",
				},
				Volumes:     []string{name + "-data:/usr/share/opensearch/data", name + "-plugins:/usr/share/opensearch/plugins"},
				Networks:    []string{"md"},
				Healthcheck: health,
			}
		}
		f.Volumes[name+"-data"] = volume{}
		f.Volumes[name+"-plugins"] = volume{}
	}
	return marshal(f)
}
