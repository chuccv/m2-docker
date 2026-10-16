package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

var errNoCommand = errors.New("no command given")

const (
	containerUID = "1000"
	webRoot      = "/var/www/html"
)

func takeSite(args []string) (string, []string) {
	if len(args) == 0 {
		return "", args
	}
	a := args[0]
	switch {
	case (a == "-s" || a == "--site") && len(args) >= 2:
		return args[1], args[2:]
	case strings.HasPrefix(a, "--site="):
		return strings.TrimPrefix(a, "--site="), args[1:]
	}
	return "", args
}

func (e *env) dockerExec(tail ...string) error {
	args := []string{"exec", "-i"}
	if e.tty() {
		args = append(args, "-t")
	}
	args = append(args, tail...)
	return e.runner.Stream("docker", args...)
}

func (e *env) execWeb(name string, rest []string, prefix ...string) error {
	s, err := e.findSite(name)
	if err != nil {
		return err
	}
	tail := []string{"-u", containerUID, "-w", webRoot, "ddev-" + s.Name + "-web"}
	tail = append(tail, prefix...)
	tail = append(tail, rest...)
	return e.dockerExec(tail...)
}

func passthrough(use, short string, e *env, prefix ...string) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Short:              short,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			name, rest := takeSite(args)
			return e.execWeb(name, rest, prefix...)
		},
	}
}

func newExecCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:                "exec [--] <command> [args...]",
		Short:              "Run a command in the web container",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			name, rest := takeSite(args)
			if len(rest) > 0 && rest[0] == "--" {
				rest = rest[1:]
			}
			if len(rest) == 0 {
				return errNoCommand
			}
			return e.execWeb(name, rest)
		},
	}
}

func newMagentoCmd(e *env) *cobra.Command {
	return passthrough("magento [args...]", "Run bin/magento in the web container", e, "php", "bin/magento")
}

func newPHPCmd(e *env) *cobra.Command {
	return passthrough("php [args...]", "Run php in the web container", e, "php")
}

func newComposerCmd(e *env) *cobra.Command {
	return passthrough("composer [args...]", "Run composer in the web container", e, "composer")
}
