package cmd

import (
	"fmt"
	osexec "os/exec"

	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/internal/ui"
)

type uiActions struct{ e *env }

func (a uiActions) Up(name string) error   { return a.e.upByName(name) }
func (a uiActions) Stop(name string) error { return a.e.stopByName(name) }
func (a uiActions) Poweroff() error        { return a.e.poweroff() }
func (a uiActions) Restart(name string) error {
	if err := a.e.stopByName(name); err != nil {
		return err
	}
	return a.e.upByName(name)
}

func newUICmd(e *env) *cobra.Command {
	var addr string
	var port int
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Serve the web dashboard on loopback",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			home, err := e.home()
			if err != nil {
				return err
			}
			l, err := ui.Listen(addr, port)
			if err != nil {
				return err
			}
			url := fmt.Sprintf("http://%s", l.Addr())
			fmt.Fprintln(e.writer(), url)
			if p, err := osexec.LookPath("xdg-open"); err == nil {
				osexec.Command(p, url).Start()
			}
			return ui.Serve(l, ui.NewServer(home, e.runner, uiActions{e}))
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1", "listen address (loopback only)")
	cmd.Flags().IntVar(&port, "port", 9900, "listen port")
	return cmd
}
