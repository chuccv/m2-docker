package cmd

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/internal/site"
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
	var all bool
	cmd := &cobra.Command{
		Use:   "hosts [site]",
		Short: "Print /etc/hosts lines needed for the current site",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			var sites []site.Site
			if all {
				for _, root := range e.scanRoots {
					sites = append(sites, site.ScanAll(root)...)
				}
			} else {
				name := ""
				if len(args) == 1 {
					name = args[0]
				}
				s, err := e.findSite(name)
				if err != nil {
					return err
				}
				sites = []site.Site{s}
			}
			for _, l := range hostsLines(sites, net.LookupHost) {
				fmt.Fprintln(c.OutOrStdout(), l)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "every site under the scan roots")
	return cmd
}
