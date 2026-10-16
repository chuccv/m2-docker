package cmd

import (
	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/internal/stack"
)

func (e *env) stopSite(home, name string, searchVersions []string) error {
	return stack.Down(e.runner, home, name, searchVersions)
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
	if key := s.SearchKey(); key != "" && !hasString(versions, key) {
		versions = append(append([]string{}, versions...), key)
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
