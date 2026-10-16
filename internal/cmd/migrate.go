package cmd

import (
	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/install"
	"github.com/chuccv/m2-docker/internal/site"
)

func newMigrateCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate [dir]",
		Short: "Migrate a DDEV site to md (.md/config.yaml)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			root, err := e.rootArg(args)
			if err != nil {
				return err
			}
			home, err := e.home()
			if err != nil {
				return err
			}
			cfg, err := config.Load(home)
			if err != nil {
				return err
			}
			return install.Migrate(root, e.runner, home, cfg.Ports, func(s site.Site) error { return e.up(s, home) })
		},
	}
}
