package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chuccv/magento2-docker/internal/config"
)

const confDir = "/usr/local/etc/php/conf.d"

func toggleScript(ext string, on bool) string {
	live := fmt.Sprintf("%s/%s.ini", confDir, ext)
	off := live + ".off"
	from, to := off, live
	if !on {
		from, to = live, off
	}
	return fmt.Sprintf("[ -e %s ] || exit 0; mv %s %s && pkill -USR2 -o php-fpm", from, from, to)
}

func (e *env) toggle(name, ext string, on bool) error {
	s, err := e.findSite(name)
	if err != nil {
		return err
	}
	if err := e.runner.Stream("docker", "exec", "-u", containerUID, "ddev-"+s.Name+"-web", "sh", "-c", toggleScript(ext, on)); err != nil {
		return err
	}
	if ext == "xhprof" && on {
		home, err := e.home()
		if err != nil {
			return err
		}
		cfg, err := config.Load(home)
		if err != nil {
			return err
		}
		fmt.Fprintf(e.out, "https://%s:%d\n", s.Hostnames[0], cfg.Ports.XHGuiHTTPS)
	}
	return nil
}

func newToggleCmd(e *env, ext string) *cobra.Command {
	return &cobra.Command{
		Use:       ext + " on|off",
		Short:     "Turn " + ext + " on or off in the web container",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"on", "off"},
		RunE: func(c *cobra.Command, args []string) error {
			return e.toggle("", ext, args[0] == "on")
		},
	}
}
