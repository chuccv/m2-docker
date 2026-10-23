package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chuccv/magento2-docker/internal/site"
)

func (e *env) addRabbitMQ(name string) error {
	s, err := e.findSite(name)
	if err != nil {
		return err
	}
	if !site.HasMDConfig(s.Root) {
		return fmt.Errorf("%s has no .md/config.yaml: run md migrate or md init first", s.Name)
	}
	c, err := site.ReadMDConfig(s.Root)
	if err != nil {
		return err
	}
	if !c.RabbitMQ {
		c.RabbitMQ = true
		if err := site.WriteMDConfig(s.Root, c); err != nil {
			return err
		}
	}
	if err := e.upByName(name); err != nil {
		return err
	}
	if s, err = site.Load(s.Root); err != nil {
		return err
	}
	if !fileExists(filepath.Join(s.Root, "app", "etc", "env.php")) {
		fmt.Fprintf(e.writer(), "RabbitMQ is running. Magento is not installed yet: add %s to setup:install\n", strings.Join(s.AMQPArgs(), " "))
		return nil
	}
	if !site.HasLocalAMQP(s.Root) {
		args := append([]string{"exec", "-u", containerUID, "-w", "/var/www/html", "ddev-" + s.Name + "-web", "php", "bin/magento", "setup:config:set", "--no-interaction"}, s.AMQPArgs()...)
		if err := e.runner.Stream("docker", args...); err != nil {
			return err
		}
	}
	fmt.Fprintf(e.writer(), "RabbitMQ added to %s (user %s)\n", s.Name, s.RabbitMQUser)
	return nil
}

func newAddonCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:       "addon rabbitmq",
		Short:     "Add a service to the site, saved in .md/config.yaml",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"rabbitmq"},
		RunE: func(c *cobra.Command, args []string) error {
			return e.addRabbitMQ("")
		},
	}
}
