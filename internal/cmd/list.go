package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"m2-docker/internal/config"
	"m2-docker/internal/site"
)

func dash(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func newListCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sites started by md",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			home, err := e.home()
			if err != nil {
				return err
			}
			cfg, err := config.Load(home)
			if err != nil {
				return err
			}
			st, err := loadState(home)
			if err != nil {
				return err
			}
			running, err := e.runningProjects()
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(e.writer(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSTATUS\tURL\tPHP\tDB\tSEARCH")
			for _, ref := range st.Sites {
				status := "stopped"
				if running["md-"+ref.Name] {
					status = "running"
				}
				s, err := site.Load(ref.Root)
				if err != nil {
					fmt.Fprintf(tw, "%s\t%s\t-\t-\t-\t-\n", ref.Name, status)
					continue
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", s.Name, status, siteURL(s.Hostnames[0], cfg.Ports.HTTPS), dash(s.PHPVersion), dash(s.DBType+" "+s.DBVersion), dash(s.SearchVersion))
			}
			return tw.Flush()
		},
	}
}
