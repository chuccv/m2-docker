package cmd

import (
	"github.com/spf13/cobra"

	"github.com/chuccv/magento2-docker/internal/site"
	"github.com/chuccv/magento2-docker/internal/stack"
)

func (e *env) stopSite(home, name string, searchVersions []string) error {
	return stack.Down(e.runner, home, name, searchVersions)
}

func (e *env) stopByName(name string) error {
	return e.stop(name, true)
}

func (e *env) restartByName(name string) error {
	if err := e.stop(name, false); err != nil {
		return err
	}
	return e.upByName(name)
}

func (e *env) stop(name string, stopSearch bool) error {
	s, err := e.findSite(name)
	if err != nil {
		return err
	}
	home, err := e.home()
	if err != nil {
		return err
	}
	unlock, err := lockHome(home, e.writer())
	if err != nil {
		return err
	}
	defer unlock()
	st, err := loadState(home)
	if err != nil {
		return err
	}
	versions := st.SearchVersions
	if key := s.SearchKey(); key != "" && !hasString(versions, key) {
		versions = append(append([]string{}, versions...), key)
	}
	if err := e.stopSite(home, s.Name, versions); err != nil {
		return err
	}
	key := s.SearchKey()
	if !stopSearch || key == "" {
		return nil
	}
	running, err := e.runningProjects()
	if err != nil {
		return err
	}
	for _, ref := range st.Sites {
		if ref.Name == s.Name || !running["md-"+ref.Name] {
			continue
		}
		if o, err := site.Load(ref.Root); err == nil && o.SearchKey() == key {
			return nil
		}
	}
	e.runner.Run("docker", "stop", site.SearchContainer(key))
	return nil
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
			return e.restartByName(nameArg(args))
		},
	}
}
