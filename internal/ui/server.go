package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/chuccv/magento2-docker/internal/run"
)

//go:embed web
var webFS embed.FS

type Actions interface {
	Up(name string) error
	Stop(name string) error
	Restart(name string) error
	Poweroff() error
}

type server struct {
	home      string
	r         run.Runner
	actions   Actions
	scanRoots []string
	mu        sync.Mutex
	busy      map[string]bool
	hub       *hub
}

var siteName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

const poweroffKey = "*poweroff*"

func NewServer(home string, r run.Runner, actions Actions) http.Handler {
	return newServer(home, r, actions, []string{"/var/www/html"})
}

func newServer(home string, r run.Runner, actions Actions, roots []string) http.Handler {
	return newServerStream(home, r, actions, roots, execStream)
}

func newServerStream(home string, r run.Runner, actions Actions, roots []string, stream streamFunc) http.Handler {
	s := &server{home: home, r: r, actions: actions, scanRoots: roots, busy: map[string]bool{}}
	s.hub = newHub(stream, s.sitesPayload)
	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(sub))
	mux.HandleFunc("GET /api/sites", s.handleSites)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /api/disk", s.handleDisk)
	mux.HandleFunc("GET /api/clean", s.handleCleanPlan)
	mux.HandleFunc("POST /api/clean", s.handleCleanApply)
	mux.HandleFunc("POST /api/poweroff", s.handlePoweroff)
	mux.HandleFunc("POST /api/sites/{name}/{action}", s.handleSiteAction)
	return guard(mux)
}

func isLoopbackHost(hostport string) bool {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		h = hostport
	}
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !isLoopbackHost(req.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if req.Method == http.MethodPost {
			if req.Header.Get("X-MD") != "1" {
				http.Error(w, "missing X-MD header", http.StatusForbidden)
				return
			}
			if o := req.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || !isLoopbackHost(u.Host) || !strings.EqualFold(u.Host, req.Host) {
					http.Error(w, "forbidden origin", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, req)
	})
}

func CheckLoopback(addr string) error {
	if addr == "localhost" {
		return nil
	}
	ip := net.ParseIP(addr)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("refusing to listen on %q: only loopback addresses are allowed", addr)
	}
	return nil
}

func Listen(addr string, port int) (net.Listener, error) {
	if err := CheckLoopback(addr); err != nil {
		return nil, err
	}
	return net.Listen("tcp", net.JoinHostPort(addr, fmt.Sprint(port)))
}

func Serve(l net.Listener, h http.Handler) error {
	return (&http.Server{Handler: h}).Serve(l)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

type result struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func respond(w http.ResponseWriter, err error) {
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, result{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result{OK: true})
}

func (s *server) acquire(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == poweroffKey {
		if len(s.busy) > 0 {
			return false
		}
	} else if s.busy[key] || s.busy[poweroffKey] {
		return false
	}
	s.busy[key] = true
	return true
}

func (s *server) release(key string) {
	s.mu.Lock()
	delete(s.busy, key)
	s.mu.Unlock()
}

func (s *server) handleSiteAction(w http.ResponseWriter, req *http.Request) {
	name, action := req.PathValue("name"), req.PathValue("action")
	if action != "up" && action != "stop" && action != "restart" {
		http.NotFound(w, req)
		return
	}
	if !siteName.MatchString(name) {
		writeJSON(w, http.StatusBadRequest, result{Error: "invalid site name"})
		return
	}
	if !s.acquire(name) {
		writeJSON(w, http.StatusConflict, result{Error: "another action is running"})
		return
	}
	defer s.release(name)
	respond(w, s.dispatch(name, action))
}

func (s *server) handlePoweroff(w http.ResponseWriter, _ *http.Request) {
	if !s.acquire(poweroffKey) {
		writeJSON(w, http.StatusConflict, result{Error: "another action is running"})
		return
	}
	defer s.release(poweroffKey)
	respond(w, s.actions.Poweroff())
}

func (s *server) dispatch(name, action string) error {
	views, err := s.siteViews()
	if err != nil {
		return err
	}
	for _, v := range views {
		if v.Name != name {
			continue
		}
		if v.Engine == "ddev" && action == "up" {
			return fmt.Errorf("%s is already running under ddev", name)
		}
		if v.Engine == "ddev" || (v.Engine == "" && v.Source == "ddev") {
			verb := map[string]string{"up": "start", "stop": "stop", "restart": "restart"}[action]
			_, err := s.r.Run("ddev", verb, name)
			return err
		}
	}
	switch action {
	case "up":
		return s.actions.Up(name)
	case "stop":
		return s.actions.Stop(name)
	}
	return s.actions.Restart(name)
}
