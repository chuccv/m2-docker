package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chuccv/magento2-docker/internal/site"
)

func cronScript(on bool) string {
	if on {
		return "pgrep -x supercronic >/dev/null || (nohup supercronic -quiet /etc/md/crontab >/proc/1/fd/1 2>&1 </dev/null &)"
	}
	return "pkill -x supercronic; exit 0"
}

func (e *env) cron(name string, on bool) error {
	s, err := e.findSite(name)
	if err != nil {
		return err
	}
	if !site.HasMDConfig(s.Root) {
		return fmt.Errorf("%s has no .md/config.yaml: run md migrate first", s.Name)
	}
	c, err := site.ReadMDConfig(s.Root)
	if err != nil {
		return err
	}
	c.Cron = on
	if err := site.WriteMDConfig(s.Root, c); err != nil {
		return err
	}
	state := "off"
	if on {
		state = "on"
	}
	container := "ddev-" + s.Name + "-web"
	out, err := e.runner.Run("docker", "ps", "--filter", "name=^"+container+"$", "--filter", "status=running", "--format", "{{.Names}}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		fmt.Fprintf(e.writer(), "cron %s for %s, applies on next md up\n", state, s.Name)
		return nil
	}
	if err := e.runner.Stream("docker", "exec", "-u", containerUID, container, "sh", "-c", cronScript(on)); err != nil {
		return err
	}
	fmt.Fprintf(e.writer(), "cron %s for %s\n", state, s.Name)
	return nil
}

func newCronCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:       "cron on|off",
		Short:     "Turn Magento cron (supercronic) on or off, saved in .md/config.yaml",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"on", "off"},
		RunE: func(c *cobra.Command, args []string) error {
			return e.cron("", args[0] == "on")
		},
	}
}
