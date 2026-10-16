package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Ports struct{ HTTP, HTTPS, MailpitHTTP, MailpitHTTPS, XHGuiHTTP, XHGuiHTTPS, PhpMyAdminHTTP, PhpMyAdminHTTPS int }

type Config struct {
	Ports             Ports
	BindAllInterfaces bool
}

type rawPorts struct {
	HTTP         int `yaml:"http"`
	HTTPS        int `yaml:"https"`
	MailpitHTTP  int `yaml:"mailpit_http"`
	MailpitHTTPS int `yaml:"mailpit_https"`
	XHGuiHTTP    int `yaml:"xhgui_http"`
	XHGuiHTTPS   int `yaml:"xhgui_https"`

	PhpMyAdminHTTP  int `yaml:"phpmyadmin_http"`
	PhpMyAdminHTTPS int `yaml:"phpmyadmin_https"`
}

func Load(home string) (Config, error) {
	c := Config{Ports: Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142, PhpMyAdminHTTP: 8036, PhpMyAdminHTTPS: 8037}}
	path := filepath.Join(home, "config.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read %s: %w", path, err)
	}
	var raw struct {
		Ports             rawPorts `yaml:"ports"`
		BindAllInterfaces bool     `yaml:"bind_all_interfaces"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	c.BindAllInterfaces = raw.BindAllInterfaces
	set := func(dst *int, v int) {
		if v != 0 {
			*dst = v
		}
	}
	set(&c.Ports.HTTP, raw.Ports.HTTP)
	set(&c.Ports.HTTPS, raw.Ports.HTTPS)
	set(&c.Ports.MailpitHTTP, raw.Ports.MailpitHTTP)
	set(&c.Ports.MailpitHTTPS, raw.Ports.MailpitHTTPS)
	set(&c.Ports.XHGuiHTTP, raw.Ports.XHGuiHTTP)
	set(&c.Ports.XHGuiHTTPS, raw.Ports.XHGuiHTTPS)
	set(&c.Ports.PhpMyAdminHTTP, raw.Ports.PhpMyAdminHTTP)
	set(&c.Ports.PhpMyAdminHTTPS, raw.Ports.PhpMyAdminHTTPS)
	return c, nil
}

func Home() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".md"), nil
}
