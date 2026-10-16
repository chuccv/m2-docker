package compose

import (
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

func RenderSite(s site.Site, p Paths) ([]byte, error) {
	managed := map[string]string{"com.md.managed": "true"}
	nodeVol := "md-node-" + nodeMajor(s.NodeVersion)
	dbVol := s.Name + "-mariadb"

	f := file{
		Services: map[string]service{
			"web": {
				Image:         "md/php:" + s.PHPVersion,
				ContainerName: "ddev-" + s.Name + "-web",
				Restart:       "unless-stopped",
				User:          "1000:1000",
				Volumes: []string{
					s.Root + ":/var/www/html",
					nodeVol + ":/opt/node:ro",
					p.Home + "/ca/rootCA.pem:/usr/local/share/ca-certificates/md.crt:ro",
					p.Home + "/sites/" + s.Name + "/nginx-run.conf:/etc/nginx/md-run.conf:ro",
				},
				ExtraHosts: []string{"host.docker.internal:host-gateway"},
				Labels:     managed,
			},
			"db": {
				Image:         "mariadb:" + s.DBVersion,
				ContainerName: "ddev-" + s.Name + "-db",
				Restart:       "unless-stopped",
				User:          "1000:1000",
				Environment:   []string{"MARIADB_USER=db", "MARIADB_PASSWORD=db", "MARIADB_DATABASE=db"},
				Volumes:       []string{dbVol + ":/var/lib/mysql"},
				Labels:        managed,
			},
		},
		Volumes: map[string]volume{
			nodeVol: {External: true},
			dbVol:   {External: true},
		},
	}
	if hasAddon(s, "redis") {
		redisVol := "ddev-" + s.Name + "_redis"
		f.Services["redis"] = service{
			Image:         "redis:7",
			ContainerName: "ddev-" + s.Name + "-redis",
			Restart:       "unless-stopped",
			Volumes:       []string{redisVol + ":/data"},
			Labels:        managed,
		}
		f.Volumes[redisVol] = volume{External: true}
	}
	return marshal(f)
}
