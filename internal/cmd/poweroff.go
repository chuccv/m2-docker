package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func (e *env) runningProjects() (map[string]bool, error) {
	out, err := e.runner.Run("docker", "compose", "ls", "--format", "json")
	if err != nil {
		return nil, err
	}
	running := map[string]bool{}
	if strings.TrimSpace(out) == "" {
		return running, nil
	}
	var rows []struct{ Name, Status string }
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if strings.HasPrefix(r.Status, "running") {
			running[r.Name] = true
		}
	}
	return running, nil
}

func (e *env) poweroff() error {
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
	running, err := e.runningProjects()
	if err != nil {
		return err
	}
	var errs []error
	for _, s := range st.Sites {
		if !running["md-"+s.Name] {
			continue
		}
		if err := e.stopSite(home, s.Name, st.SearchVersions); err != nil {
			errs = append(errs, fmt.Errorf("stop %s: %w", s.Name, err))
		}
	}
	if running["md-shared"] {
		if err := e.runner.Stream("docker", "compose", "-p", "md-shared", "-f", filepath.Join(home, "shared", "compose.yaml"), "down"); err != nil {
			errs = append(errs, fmt.Errorf("stop shared: %w", err))
		}
	}
	return errors.Join(errs...)
}

func newPoweroffCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "poweroff",
		Short: "Stop every running site and the shared stack",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return e.poweroff()
		},
	}
}
