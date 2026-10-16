package site

import (
	"errors"
	"fmt"
	"strings"
)

func (s Site) HasAddon(name string) bool { return contains(s.Addons, name) }

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
	if strings.HasPrefix(s.Webserver, "apache") {
		errs = append(errs, fmt.Errorf("webserver %q (apache) is not supported, only nginx-fpm", s.Webserver))
	}
	for _, a := range []string{"elasticsearch", "varnish", "rabbitmq"} {
		if contains(s.Addons, a) {
			errs = append(errs, fmt.Errorf("addon %s is not supported", a))
		}
	}
	return errors.Join(errs...)
}
