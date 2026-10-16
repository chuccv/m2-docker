package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"m2-docker/internal/clean"
	"m2-docker/internal/site"
	"m2-docker/internal/snapshot"
)

var errAborted = errors.New("aborted")

func (e *env) confirm(prompt string) bool {
	fmt.Fprint(e.out, prompt)
	if e.in == nil {
		return false
	}
	line, _ := bufio.NewReader(e.in).ReadString('\n')
	return strings.TrimSpace(line) == "yes"
}

func newSnapshotCmd(e *env) *cobra.Command {
	var label string
	var list bool
	c := &cobra.Command{
		Use:   "snapshot",
		Short: "Create or list database snapshots",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			s, err := e.findSite("")
			if err != nil {
				return err
			}
			if list {
				files, err := snapshot.List(s)
				if err != nil {
					return err
				}
				for _, f := range files {
					fmt.Fprintln(e.out, f)
				}
				return nil
			}
			p, err := snapshot.Create(s, e.runner, label)
			if err != nil {
				return err
			}
			fmt.Fprintln(e.out, p)
			return nil
		},
	}
	c.Flags().StringVar(&label, "label", "", "snapshot label (default: timestamp)")
	c.Flags().BoolVar(&list, "list", false, "list snapshots")

	var yes bool
	restore := &cobra.Command{
		Use:   "restore <file>",
		Short: "Restore a snapshot into the site database",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := e.findSite("")
			if err != nil {
				return err
			}
			prompt := fmt.Sprintf("Restore %s into %s? This drops database db. Type yes: ", args[0], s.Name)
			if !strings.HasPrefix(filepath.Base(args[0]), s.Name+"-") {
				prompt = fmt.Sprintf("WARNING: %s does not look like a snapshot of %s.\n", filepath.Base(args[0]), s.Name) + prompt
			}
			if !yes && !e.confirm(prompt) {
				return errAborted
			}
			return snapshot.Restore(s, e.runner, args[0])
		},
	}
	restore.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	c.AddCommand(restore)
	return c
}

func newCleanCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Delete unused docker images, build cache and volumes",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			var sites []site.Site
			if s, err := e.findSite(""); err == nil {
				sites = append(sites, s)
			}
			items, err := clean.Plan(e.runner, sites)
			if err != nil {
				return err
			}
			var del, orphans []clean.Item
			for _, it := range items {
				if it.Kind == "db-orphan" {
					orphans = append(orphans, it)
				} else {
					del = append(del, it)
				}
			}
			for _, kind := range []string{"image", "buildcache", "volume"} {
				for _, it := range del {
					if it.Kind == kind {
						fmt.Fprintf(e.out, "%-10s %s %s\n", it.Kind, it.Name, it.Size)
					}
				}
			}
			if len(orphans) > 0 {
				fmt.Fprintln(e.out, "DB mồ côi — xoá tay:")
				for _, it := range orphans {
					fmt.Fprintf(e.out, "  docker volume rm %s\n", it.Name)
				}
			}
			if len(del) == 0 {
				return nil
			}
			if !e.confirm("Type yes to delete: ") {
				return nil
			}
			return clean.Apply(del, e.runner)
		},
	}
}
