package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/internal/config"
)

func newDescribeCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "describe [name]",
		Short: "Show containers, hostnames, database credentials and ports of a site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := e.findSite(nameArg(args))
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
			running, err := e.runningProjects()
			if err != nil {
				return err
			}
			status := "stopped"
			if running["md-"+s.Name] {
				status = "running"
			}
			names, _ := e.runner.Run("docker", "ps", "-a", "--filter", "label=com.docker.compose.project=md-"+s.Name, "--format", "{{.Names}}")
			w := e.writer()
			fmt.Fprintf(w, "Name:       %s\n", s.Name)
			fmt.Fprintf(w, "Status:     %s\n", status)
			fmt.Fprintf(w, "Root:       %s\n", s.Root)
			fmt.Fprintf(w, "PHP:        %s\n", dash(s.PHPVersion))
			fmt.Fprintf(w, "Database:   %s %s\n", s.DBType, s.DBVersion)
			fmt.Fprintf(w, "Search:     %s\n", dash(s.SearchVersion))
			fmt.Fprintf(w, "DB creds:   db/db/db (user/password/database), host db\n")
			fmt.Fprintf(w, "Ports:      http %d, https %d, mailpit %d/%d, xhgui %d/%d\n", cfg.Ports.HTTP, cfg.Ports.HTTPS, cfg.Ports.MailpitHTTP, cfg.Ports.MailpitHTTPS, cfg.Ports.XHGuiHTTP, cfg.Ports.XHGuiHTTPS)
			fmt.Fprintln(w, "Hostnames:")
			for _, h := range s.Hostnames {
				fmt.Fprintf(w, "  %s\n", siteURL(h, cfg.Ports.HTTPS))
			}
			fmt.Fprintln(w, "Containers:")
			for _, n := range strings.Fields(names) {
				fmt.Fprintf(w, "  %s\n", n)
			}
			return nil
		},
	}
}
