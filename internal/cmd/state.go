package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

type siteRef struct {
	Name string `json:"name"`
	Root string `json:"root"`
}

type state struct {
	SearchVersions []string  `json:"search_versions"`
	Sites          []siteRef `json:"sites"`
}

func statePath(home string) string { return filepath.Join(home, "state.json") }

func siteFile(home, name string) string { return filepath.Join(home, "sites", name, "compose.yaml") }

func loadState(home string) (state, error) {
	var st state
	data, err := os.ReadFile(statePath(home))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(data, &st)
}

func saveState(home string, st state) error {
	sort.Strings(st.SearchVersions)
	sort.Slice(st.Sites, func(i, j int) bool { return st.Sites[i].Name < st.Sites[j].Name })
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	return os.WriteFile(statePath(home), data, 0o644)
}

func lockHome(home string, w io.Writer) (func(), error) {
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(home, "md.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fmt.Fprintln(w, "đang chờ lệnh md khác chạy xong...")
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			f.Close()
			return nil, err
		}
	}
	return func() { f.Close() }, nil
}

func (st *state) add(s siteRef, search string) {
	if search != "" && !hasString(st.SearchVersions, search) {
		st.SearchVersions = append(st.SearchVersions, search)
	}
	for i := range st.Sites {
		if st.Sites[i].Name == s.Name {
			st.Sites[i] = s
			return
		}
	}
	st.Sites = append(st.Sites, s)
}

func hasString(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

func (e *env) writer() io.Writer {
	if e.out == nil {
		return io.Discard
	}
	return e.out
}
