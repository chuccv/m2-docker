package cmd

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"m2-docker/internal/site"
)

func hostsLines(sites []site.Site, lookup func(string) ([]string, error)) []string {
	seen := map[string]bool{}
	var lines []string
	for _, s := range sites {
		for _, h := range s.Hostnames {
			if strings.HasSuffix(h, ".ddev.site") || seen[h] {
				continue
			}
			seen[h] = true
			addrs, err := lookup(h)
			if err == nil && containsStr(addrs, "127.0.0.1") {
				continue
			}
			lines = append(lines, "127.0.0.1 "+h)
		}
	}
	sort.Strings(lines)
	return lines
}

func containsStr(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

func newHostsCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "hosts [site]",
		Short: "Print /etc/hosts lines needed for the current site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			s, err := e.findSite(name)
			if err != nil {
				return err
			}
			for _, l := range hostsLines([]site.Site{s}, net.LookupHost) {
				fmt.Fprintln(c.OutOrStdout(), l)
			}
			return nil
		},
	}
}
