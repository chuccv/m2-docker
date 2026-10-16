package cmd

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

type env struct {
	runner    run.Runner
	tty       func() bool
	scanRoots []string
	getwd     func() (string, error)
	home      func() (string, error)
	out       io.Writer
	in        io.Reader
	site      string
}

func defaultEnv() *env {
	return &env{
		runner:    run.ExecRunner{},
		tty:       stdioIsTerminal,
		scanRoots: []string{"/var/www/html"},
		getwd:     os.Getwd,
		home:      config.Home,
		out:       os.Stdout,
		in:        os.Stdin,
	}
}

func stdioIsTerminal() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

func isCharDevice(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func (e *env) findSite(name string) (site.Site, error) {
	if name == "" {
		name = e.site
	}
	wd, err := e.getwd()
	if err != nil {
		return site.Site{}, err
	}
	return site.Find(name, wd, e.scanRoots)
}

func register(root *cobra.Command, e *env) {
	root.PersistentFlags().StringVarP(&e.site, "site", "s", "", "site name (default: from current directory)")
	root.AddCommand(newTrustCmd(), newHostsCmd(e))
	root.AddCommand(newUpCmd(e), newStopCmd(e), newRestartCmd(e), newPoweroffCmd(e), newListCmd(e), newDescribeCmd(e))
	root.AddCommand(newExecCmd(e), newMagentoCmd(e), newPHPCmd(e), newComposerCmd(e), newMySQLCmd(e))
	root.AddCommand(newToggleCmd(e, "xdebug"), newToggleCmd(e, "xhprof"))
	root.AddCommand(newSnapshotCmd(e), newCleanCmd(e), newInstallCmd(e), newMigrateCmd(e), newUICmd(e))
}

func Register(root *cobra.Command) {
	register(root, defaultEnv())
}
