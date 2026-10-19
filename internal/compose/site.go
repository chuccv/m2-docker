package compose

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/chuccv/m2-docker/internal/site"
)

func nodeMajor(v string) string {
	major, _, _ := strings.Cut(v, ".")
	return major
}

func hasAddon(s site.Site, name string) bool {
	for _, a := range s.Addons {
		if a == name {
			return true
		}
	}
	return false
}

func ComposerAuth() string {
	var candidates []string
	if h := os.Getenv("COMPOSER_HOME"); h != "" {
		candidates = append(candidates, filepath.Join(h, "auth.json"))
	}
	if h, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(h, ".composer", "auth.json"), filepath.Join(h, ".config", "composer", "auth.json"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func dbService(s site.Site, vol string, labels map[string]string) service {
	svc := service{
		Image:         s.DBImage(),
		ContainerName: "ddev-" + s.Name + "-db",
		Restart:       "no",
		User:          "1000:1000",
		Volumes:       []string{vol + ":/var/lib/mysql"},
		Labels:        labels,
	}
	if s.DBType == "mysql" {
		svc.Environment = []string{"MYSQL_USER=db", "MYSQL_PASSWORD=db", "MYSQL_DATABASE=db", "MYSQL_ROOT_PASSWORD=root"}
	} else {
		svc.Environment = []string{"MARIADB_USER=db", "MARIADB_PASSWORD=db", "MARIADB_DATABASE=db"}
	}
	return svc
}

func RenderSite(s site.Site, p Paths) ([]byte, error) {
	managed := map[string]string{"com.md.managed": "true"}
	nodeVol := "md-node-" + nodeMajor(s.NodeVersion)
	dbVol := s.DBVolume()

	f := file{
		Services: map[string]service{
			"web": {
				Image:         "md/php:" + s.PHPVersion,
				ContainerName: "ddev-" + s.Name + "-web",
				Restart:       "no",
				User:          "1000:1000",
				Volumes: []string{
					s.Root + ":/var/www/html",
					nodeVol + ":/opt/node:ro",
					p.Home + "/ca/rootCA.pem:/usr/local/share/ca-certificates/md.crt:ro",
				},
				ExtraHosts: []string{"host.docker.internal:host-gateway"},
				Labels:     managed,
			},
			"db": dbService(s, dbVol, managed),
		},
		Volumes: map[string]volume{
			nodeVol: {External: true},
			dbVol:   {External: true},
		},
	}
	web := f.Services["web"]
	if s.IsApache() {
		web.Volumes = append(web.Volumes, p.Home+"/sites/"+s.Name+"/apache-run.conf:/etc/apache2/md-run.conf:ro")
		web.Environment = append(web.Environment, "MD_WEBSERVER=apache")
	} else {
		web.Volumes = append(web.Volumes, p.Home+"/sites/"+s.Name+"/nginx-run.conf:/etc/nginx/md-run.conf:ro")
	}
	f.Services["web"] = web
	if p.ComposerAuth != "" {
		web := f.Services["web"]
		web.Volumes = append(web.Volumes, p.ComposerAuth+":/home/www/.composer/auth.json:ro")
		web.Environment = append(web.Environment, "COMPOSER_HOME=/home/www/.composer", "COMPOSER_CACHE_DIR=/tmp/composer-cache")
		f.Services["web"] = web
	}
	if hasAddon(s, "redis") {
		redisVol := "ddev-" + s.Name + "_redis"
		f.Services["redis"] = service{
			Image:         "redis:7",
			ContainerName: "ddev-" + s.Name + "-redis",
			Restart:       "no",
			Volumes:       []string{redisVol + ":/data"},
			Labels:        managed,
		}
		f.Volumes[redisVol] = volume{External: true}
	}
	if s.Varnish != "" {
		f.Services["varnish"] = service{
			Image:         "varnish:" + s.Varnish,
			ContainerName: "ddev-" + s.Name + "-varnish",
			Restart:       "no",
			DependsOn:     []string{"web"},
			Volumes:       []string{p.Home + "/sites/" + s.Name + "/default.vcl:/etc/varnish/default.vcl:ro"},
			Labels:        managed,
		}
	}
	if s.PhpMyAdmin {
		f.Services["phpmyadmin"] = service{
			Image:         "phpmyadmin:5",
			ContainerName: "ddev-" + s.Name + "-phpmyadmin",
			Restart:       "no",
			DependsOn:     []string{"db"},
			Environment:   []string{"PMA_HOST=db", "PMA_USER=db", "PMA_PASSWORD=db", "UPLOAD_LIMIT=1G"},
			Labels:        managed,
		}
	}
	return marshal(f)
}
