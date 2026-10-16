package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"m2-docker/internal/config"
	"m2-docker/internal/run"
	"m2-docker/internal/tls"
)

func newTrustCmd() *cobra.Command {
	var remote string
	c := &cobra.Command{
		Use:   "trust",
		Short: "Install the mkcert CA locally (and optionally on a remote host)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			home, err := config.Home()
			if err != nil {
				return err
			}
			hint, err := tls.Trust(home, remote, run.ExecRunner{})
			if err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), hint)
			return nil
		},
	}
	c.Flags().StringVar(&remote, "remote", "", "also trust the CA on a remote host via ssh (user@host or ssh config alias)")
	return c
}
