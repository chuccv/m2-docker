package cmd

import "github.com/spf13/cobra"

func newMySQLCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:                "mysql [args...]",
		Short:              "Open the mysql client in the db container",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			name, rest := takeSite(args)
			s, err := e.findSite(name)
			if err != nil {
				return err
			}
			tail := append([]string{"ddev-" + s.Name + "-db", "mysql", "-udb", "-pdb", "db"}, rest...)
			return e.dockerExec(tail...)
		},
	}
}
