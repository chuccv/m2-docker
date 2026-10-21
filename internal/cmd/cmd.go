package cmd

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/chuccv/magento2-docker/internal/config"
	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
)

type env struct {
	runner    run.Runner
	tty       func() bool
	scanRoots []string
	getwd     func() (string, error)
	home      func() (string, error)
	out       io.Writer
	in        io.Reader
	errOut    io.Writer
	lookup    func(string) ([]string, error)
	httpGet   func(string) (int, string, error)
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
		errOut:    os.Stderr,
	}
}

func stdioIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
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
	root.PersistentPreRunE = func(*cobra.Command, []string) error {
		home, err := e.home()
		if err != nil {
			return err
		}
		cfg, err := config.Load(home)
		if err != nil {
			return err
		}
		site.Domain = cfg.Domain
		return nil
	}
	root.PersistentFlags().StringVarP(&e.site, "site", "s", "", "site name (default: from current directory)")
	root.AddCommand(newTrustCmd(), newHostsCmd(e))
	root.AddCommand(newUpCmd(e), newStopCmd(e), newRestartCmd(e), newPoweroffCmd(e), newListCmd(e), newDescribeCmd(e))
	root.AddCommand(newExecCmd(e), newMagentoCmd(e), newPHPCmd(e), newComposerCmd(e), newMySQLCmd(e))
	root.AddCommand(newToggleCmd(e, "xdebug"), newToggleCmd(e, "xhprof"))
	root.AddCommand(newSnapshotCmd(e), newCleanCmd(e), newInstallCmd(e), newMigrateCmd(e), newInitCmd(e), newUICmd(e), newMCPCmd(e))
}

func Register(root *cobra.Command) {
	register(root, defaultEnv())
}
