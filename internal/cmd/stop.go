package cmd

import (
	"github.com/spf13/cobra"
)

func (e *env) stopSite(home, name string, searchVersions []string) error {
	net := "md-" + name + "_default"
	targets := []string{"md-router", "md-xhgui"}
	for _, v := range searchVersions {
		targets = append(targets, "md-opensearch-"+v)
	}
	for _, c := range targets {
		e.runner.Run("docker", "network", "disconnect", net, c)
	}
	return e.runner.Stream("docker", "compose", "-p", "md-"+name, "-f", siteFile(home, name), "down")
}

func (e *env) stopByName(name string) error {
	s, err := e.findSite(name)
	if err != nil {
		return err
	}
	home, err := e.home()
	if err != nil {
		return err
	}
	st, err := loadState(home)
	if err != nil {
		return err
	}
	versions := st.SearchVersions
	if s.SearchVersion != "" && !hasString(versions, s.SearchVersion) {
		versions = append(append([]string{}, versions...), s.SearchVersion)
	}
	return e.stopSite(home, s.Name, versions)
}

func newStopCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "stop [name]",
		Short: "Stop a site (data volumes are kept)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return e.stopByName(nameArg(args))
		},
	}
}

func newRestartCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "restart [name]",
		Short: "Stop then start a site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := nameArg(args)
			if err := e.stopByName(name); err != nil {
				return err
			}
			return e.upByName(name)
		},
	}
}
