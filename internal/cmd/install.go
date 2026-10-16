package cmd

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/install"
	"github.com/chuccv/m2-docker/internal/site"
)

func newInstallCmd(e *env) *cobra.Command {
	var o install.Options
	cmd := &cobra.Command{
		Use:   "install [dir]",
		Short: "Install a fresh Magento into a folder",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			wd, err := e.getwd()
			if err != nil {
				return err
			}
			o.Root = wd
			if len(args) > 0 {
				o.Root = args[0]
				if !filepath.IsAbs(o.Root) {
					o.Root = filepath.Join(wd, o.Root)
				}
			}
			home, err := e.home()
			if err != nil {
				return err
			}
			cfg, err := config.Load(home)
			if err != nil {
				return err
			}
			o.Out = e.writer()
			o.Up = func(s site.Site) error { return e.up(s, home, true) }
			return install.Install(o, e.runner, home, cfg.Ports)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Version, "version", "2.4.8", "Magento version")
	f.StringVar(&o.Edition, "edition", "community", "community or enterprise")
	f.StringVar(&o.PHP, "php", "", "PHP version (default: from Magento version)")
	f.StringVar(&o.DB, "db", "", "MariaDB version (default: from Magento version)")
	f.StringVar(&o.Search, "search", "", "OpenSearch version (default: from Magento version)")
	f.BoolVar(&o.SampleData, "sample-data", false, "deploy sample data")
	f.StringVar(&o.AdminUser, "admin-user", "admin", "admin user")
	f.StringVar(&o.AdminPassword, "admin-password", "admin123", "admin password")
	f.StringVar(&o.AdminEmail, "admin-email", "admin@example.com", "admin email")
	return cmd
}
