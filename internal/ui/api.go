package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/chuccv/m2-docker/internal/clean"
	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

type SiteView struct {
	Name    string            `json:"name"`
	Root    string            `json:"root"`
	Source  string            `json:"source"`
	Status  string            `json:"status"`
	PHP     string            `json:"php"`
	DB      string            `json:"db"`
	Search  string            `json:"search"`
	URLs    map[string]string `json:"urls"`
	Blocked string            `json:"blocked"`
}

func (s *server) discover() []site.Site {
	var out []site.Site
	seen := map[string]bool{}
	add := func(x site.Site) {
		if !seen[x.Name] {
			seen[x.Name] = true
			out = append(out, x)
		}
	}
	for _, ref := range readState(s.home) {
		if x, err := site.Load(ref.Root); err == nil {
			add(x)
		} else {
			add(site.Site{Name: ref.Name, Root: ref.Root, Type: "error: " + err.Error()})
		}
	}
	for _, root := range s.scanRoots {
		for _, x := range site.ScanAll(root) {
			add(x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func readState(home string) []struct{ Name, Root string } {
	data, err := readFile(home, "state.json")
	if err != nil {
		return nil
	}
	var st struct {
		Sites []struct {
			Name string `json:"name"`
			Root string `json:"root"`
		} `json:"sites"`
	}
	if json.Unmarshal(data, &st) != nil {
		return nil
	}
	out := make([]struct{ Name, Root string }, 0, len(st.Sites))
	for _, x := range st.Sites {
		out = append(out, struct{ Name, Root string }{x.Name, x.Root})
	}
	return out
}

func hostURL(host string, port int) string {
	if port == 443 {
		return "https://" + host
	}
	return fmt.Sprintf("https://%s:%d", host, port)
}

const psFormat = `{{.Label "com.md.managed"}}|{{.Label "com.docker.compose.project"}}`

func (s *server) projects() map[string]bool {
	out, err := s.r.Run("docker", "ps", "--format", psFormat)
	set := map[string]bool{}
	if err != nil {
		return set
	}
	for _, l := range strings.Fields(out) {
		managed, project, _ := strings.Cut(l, "|")
		if managed == "true" && strings.HasPrefix(project, "md-") {
			set["md:"+strings.TrimPrefix(project, "md-")] = true
		} else if strings.HasPrefix(project, "ddev-") {
			set["ddev:"+strings.TrimPrefix(project, "ddev-")] = true
		}
	}
	return set
}

func (s *server) siteViews() ([]SiteView, error) {
	cfg, err := config.Load(s.home)
	if err != nil {
		return nil, err
	}
	running := s.projects()
	views := []SiteView{}
	for _, x := range s.discover() {
		v := SiteView{Name: x.Name, Root: x.Root, Source: "ddev", Status: "stopped", PHP: x.PHPVersion, Search: x.SearchVersion, URLs: map[string]string{}}
		if site.HasMDConfig(x.Root) {
			v.Source = "md"
		}
		if x.DBType != "" {
			v.DB = strings.TrimSpace(x.DBType + " " + x.DBVersion)
		}
		if len(x.Hostnames) > 0 {
			h := x.Hostnames[0]
			v.URLs["storefront"] = hostURL(h, cfg.Ports.HTTPS)
			v.URLs["admin"] = v.URLs["storefront"] + "/admin"
			v.URLs["mailpit"] = fmt.Sprintf("https://%s:%d", h, cfg.Ports.MailpitHTTPS)
			v.URLs["xhgui"] = fmt.Sprintf("https://%s:%d", h, cfg.Ports.XHGuiHTTPS)
		}
		switch {
		case running["md:"+x.Name]:
			v.Status = "running"
		case running["ddev:"+x.Name]:
			v.Status = "ddev"
		}
		if err := site.Supported(x); err != nil {
			v.Blocked = strings.ReplaceAll(err.Error(), "\n", "; ")
		}
		if v.Status == "ddev" {
			v.Blocked = strings.TrimPrefix(v.Blocked+"; DDEV containers present, run ddev stop first", "; ")
		}
		views = append(views, v)
	}
	return views, nil
}

func (s *server) handleSites(w http.ResponseWriter, _ *http.Request) {
	v, err := s.siteViews()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, result{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) handleStats(w http.ResponseWriter, _ *http.Request) {
	var known []string
	for _, x := range s.discover() {
		known = append(known, x.Name)
	}
	st, err := collectStats(s.r, known)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, result{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

type diskImage struct {
	Repo       string `json:"repo"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	Containers string `json:"containers"`
}

type diskVolume struct {
	Name string `json:"name"`
	Size string `json:"size"`
	Site string `json:"site"`
}

func volumeSite(name string) string {
	if strings.HasPrefix(name, "md-") {
		return ""
	}
	if i := strings.Index(name, "-mariadb"); i > 0 {
		return name[:i]
	}
	return ""
}

func (s *server) handleDisk(w http.ResponseWriter, _ *http.Request) {
	out, err := s.r.Run("docker", "system", "df", "-v", "--format", "{{json .}}")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, result{Error: err.Error()})
		return
	}
	var raw struct {
		Images  []struct{ Repository, Tag, Size, Containers string }
		Volumes []struct{ Name, Size string }
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		writeJSON(w, http.StatusInternalServerError, result{Error: err.Error()})
		return
	}
	res := struct {
		Images  []diskImage  `json:"images"`
		Volumes []diskVolume `json:"volumes"`
	}{[]diskImage{}, []diskVolume{}}
	for _, i := range raw.Images {
		res.Images = append(res.Images, diskImage{i.Repository, i.Tag, i.Size, i.Containers})
	}
	for _, v := range raw.Volumes {
		res.Volumes = append(res.Volumes, diskVolume{v.Name, v.Size, volumeSite(v.Name)})
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) cleanPlan() ([]clean.Item, []clean.Item, error) {
	items, err := clean.Plan(s.r, s.discover())
	if err != nil {
		return nil, nil, err
	}
	var del, orphans []clean.Item
	for _, it := range items {
		if it.Kind == "db-orphan" {
			orphans = append(orphans, it)
		} else {
			del = append(del, it)
		}
	}
	return del, orphans, nil
}

func (s *server) handleCleanPlan(w http.ResponseWriter, _ *http.Request) {
	del, orphans, err := s.cleanPlan()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, result{Error: err.Error()})
		return
	}
	if del == nil {
		del = []clean.Item{}
	}
	if orphans == nil {
		orphans = []clean.Item{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": del, "orphans": orphans})
}

func (s *server) handleCleanApply(w http.ResponseWriter, req *http.Request) {
	var body struct{ Confirm string }
	if json.NewDecoder(req.Body).Decode(&body) != nil || body.Confirm != "yes" {
		writeJSON(w, http.StatusBadRequest, result{Error: "type yes to confirm"})
		return
	}
	if !s.acquire("*clean*") {
		writeJSON(w, http.StatusConflict, result{Error: "another action is running"})
		return
	}
	defer s.release("*clean*")
	del, _, err := s.cleanPlan()
	if err == nil {
		err = clean.Apply(del, run.Runner(s.r))
	}
	respond(w, err)
}
