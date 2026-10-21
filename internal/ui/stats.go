package ui

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/chuccv/magento2-docker/internal/run"
)

type Usage struct {
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu"`
	MemBytes   int64   `json:"mem"`
}

type Stats struct {
	Sites      []Usage `json:"sites"`
	Shared     Usage   `json:"shared"`
	Containers []Usage `json:"containers"`
}

var memUnits = []struct {
	suffix string
	mult   float64
}{
	{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
	{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3}, {"B", 1},
}

func parseMem(s string) int64 {
	used, _, _ := strings.Cut(s, "/")
	used = strings.TrimSpace(used)
	for _, u := range memUnits {
		if strings.HasSuffix(used, u.suffix) {
			v, err := strconv.ParseFloat(strings.TrimSuffix(used, u.suffix), 64)
			if err != nil {
				return 0
			}
			return int64(v * u.mult)
		}
	}
	return 0
}

func parseCPU(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	return v
}

func trimRuntime(container string) string {
	for _, p := range []string{"ddev-", "md-"} {
		if strings.HasPrefix(container, p) {
			return strings.TrimPrefix(container, p)
		}
	}
	return container
}

func isShared(rest string) bool {
	return rest == "router" || rest == "xhgui" || rest == "ssh-agent" || rest == "shared" || strings.HasPrefix(rest, "opensearch-") || strings.HasPrefix(rest, "elasticsearch-") || strings.HasPrefix(rest, "shared-")
}

func siteOf(container string, known []string) (string, bool) {
	rest := trimRuntime(container)
	if isShared(rest) {
		return "", false
	}
	best := ""
	for _, k := range known {
		if strings.HasPrefix(rest, k+"-") && len(k) > len(best) {
			best = k
		}
	}
	if best != "" {
		return best, true
	}
	i := strings.LastIndex(rest, "-")
	if i <= 0 {
		return "", false
	}
	rest = rest[:i]
	if n, err := strconv.Atoi(rest[strings.LastIndex(rest, "-")+1:]); err == nil && n > 0 {
		if j := strings.LastIndex(rest, "-"); j > 0 {
			rest = rest[:j]
		}
	}
	return rest, true
}

type statRow struct{ Name, CPUPerc, MemUsage string }

func parseStatRow(l string) (statRow, bool) {
	i := strings.Index(l, "{")
	if i < 0 {
		return statRow{}, false
	}
	var row statRow
	if json.Unmarshal([]byte(l[i:]), &row) != nil {
		return statRow{}, false
	}
	return row, true
}

func buildStats(rows []statRow, known []string) Stats {
	st := Stats{Sites: []Usage{}, Containers: []Usage{}, Shared: Usage{Name: "shared"}}
	bySite := map[string]*Usage{}
	for _, row := range rows {
		u := Usage{Name: row.Name, CPUPercent: parseCPU(row.CPUPerc), MemBytes: parseMem(row.MemUsage)}
		st.Containers = append(st.Containers, u)
		name, ok := siteOf(row.Name, known)
		if !ok {
			st.Shared.CPUPercent += u.CPUPercent
			st.Shared.MemBytes += u.MemBytes
			continue
		}
		g := bySite[name]
		if g == nil {
			g = &Usage{Name: name}
			bySite[name] = g
		}
		g.CPUPercent += u.CPUPercent
		g.MemBytes += u.MemBytes
	}
	for _, g := range bySite {
		st.Sites = append(st.Sites, *g)
	}
	sort.Slice(st.Sites, func(i, j int) bool { return st.Sites[i].Name < st.Sites[j].Name })
	return st
}

func collectStats(r run.Runner, known []string) (Stats, error) {
	out, err := r.Run("docker", "stats", "--no-stream", "--format", "{{json .}}")
	if err != nil {
		return buildStats(nil, known), err
	}
	var rows []statRow
	for _, l := range strings.Split(out, "\n") {
		if row, ok := parseStatRow(strings.TrimSpace(l)); ok {
			rows = append(rows, row)
		}
	}
	return buildStats(rows, known), nil
}
