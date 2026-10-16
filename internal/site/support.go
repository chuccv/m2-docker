package site

import (
	"errors"
	"fmt"
)

func Supported(s Site) error {
	var errs []error
	if s.Name == "shared" {
		errs = append(errs, fmt.Errorf("site name %q is reserved", s.Name))
	}
	if s.Type != "magento2" {
		errs = append(errs, fmt.Errorf("project type %q is not supported, only magento2", s.Type))
	}
	if s.DBType != "mariadb" {
		errs = append(errs, fmt.Errorf("database %q: mysql chưa hỗ trợ ở V1, only mariadb", s.DBType))
	}
	if s.Webserver != "" && s.Webserver != "nginx-fpm" && s.Webserver != "apache-fpm" {
		errs = append(errs, fmt.Errorf("webserver %q is not supported, only nginx-fpm or apache-fpm", s.Webserver))
	}
	for _, a := range []string{"elasticsearch", "varnish", "rabbitmq", "mutagen"} {
		if contains(s.Addons, a) {
			errs = append(errs, fmt.Errorf("addon %s is not supported", a))
		}
	}
	return errors.Join(errs...)
}
