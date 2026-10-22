package site

import (
	"errors"
	"fmt"
	"strings"
)

func (s Site) HasAddon(name string) bool { return contains(s.Addons, name) }

func (s Site) SearchKey() string {
	if s.SearchVersion == "" {
		return ""
	}
	if s.SearchEngine == "elasticsearch" {
		return "elasticsearch:" + s.SearchVersion
	}
	return s.SearchVersion
}

func SearchContainer(key string) string {
	if v, ok := strings.CutPrefix(key, "elasticsearch:"); ok {
		return "md-elasticsearch-" + v
	}
	return "md-opensearch-" + key
}

func SearchAlias(key string) string {
	if strings.HasPrefix(key, "elasticsearch:") {
		return "elasticsearch"
	}
	return "opensearch"
}

func (s Site) isMySQL() bool { return s.DBType == "mysql" }

func (s Site) DBVolume() string {
	if s.isMySQL() {
		return s.Name + "-mysql"
	}
	return s.Name + "-mariadb"
}

func (s Site) RedisVolume() string {
	if s.DDEV {
		return "ddev-" + s.Name + "_redis"
	}
	return s.Name + "-redis"
}

func (s Site) DBClient() string {
	if s.isMySQL() {
		return "mysql"
	}
	return "mariadb"
}

func (s Site) DBDump() string {
	if s.isMySQL() {
		return "mysqldump"
	}
	return "mariadb-dump"
}

func (s Site) DBVersionFile() string {
	return "db_" + s.DBClient() + "_version.txt"
}

func (s Site) DBLabel() string {
	return s.DBClient() + "_" + s.DBVersion
}

func (s Site) DBImage() string {
	return s.DBClient() + ":" + s.DBVersion
}

func Supported(s Site) error {
	var errs []error
	if s.Name == "shared" {
		errs = append(errs, fmt.Errorf("site name %q is reserved", s.Name))
	}
	if s.Type != "magento2" {
		errs = append(errs, fmt.Errorf("project type %q is not supported, only magento2", s.Type))
	}
	if s.DBType != "mariadb" && s.DBType != "mysql" {
		errs = append(errs, fmt.Errorf("database %q is not supported, only mariadb or mysql", s.DBType))
	}
	if s.Webserver != "" && s.Webserver != "nginx-fpm" && s.Webserver != "apache-fpm" {
		errs = append(errs, fmt.Errorf("webserver %q is not supported, only nginx-fpm or apache-fpm", s.Webserver))
	}
	if s.SearchEngine != "" && s.SearchEngine != "opensearch" && s.SearchEngine != "elasticsearch" {
		errs = append(errs, fmt.Errorf("search_engine %q is not supported, only opensearch or elasticsearch", s.SearchEngine))
	}
	if s.SearchVersion == "latest" {
		errs = append(errs, fmt.Errorf("search version \"latest\" is not supported, pin a version in .md/config.yaml"))
	}
	for _, a := range []string{"rabbitmq"} {
		if contains(s.Addons, a) {
			errs = append(errs, fmt.Errorf("addon %s is not supported", a))
		}
	}
	return errors.Join(errs...)
}
