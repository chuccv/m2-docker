package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/chuccv/magento2-docker/internal/cmd"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:           "md",
		Short:         "Local Magento 2 environment tool",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Register(root)
	root.SetVersionTemplate("md {{.Version}}\n")
	if err := root.Execute(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if err.Error() != ee.Error() {
				fmt.Fprintln(os.Stderr, err)
			}
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
