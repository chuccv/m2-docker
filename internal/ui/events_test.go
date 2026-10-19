package ui

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chuccv/m2-docker/internal/run"
)

type fakeStream struct {
	mu      sync.Mutex
	started map[string]int
	stopped map[string]int
	lines   map[string][]string
}

func newFakeStream(lines map[string][]string) *fakeStream {
	return &fakeStream{started: map[string]int{}, stopped: map[string]int{}, lines: lines}
}

func (f *fakeStream) fn(ctx context.Context, onLine func(string), name string, args ...string) error {
	key := args[0]
	f.mu.Lock()
	f.started[key]++
	f.mu.Unlock()
	for _, l := range f.lines[key] {
		onLine(l)
	}
	<-ctx.Done()
	f.mu.Lock()
	f.stopped[key]++
	f.mu.Unlock()
	return ctx.Err()
}

func (f *fakeStream) count(m map[string]int, key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return m[key]
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met")
}

func TestHubLifecycle(t *testing.T) {
	f := newFakeStream(nil)
	h := newHub(f.fn, func() ([]byte, []string) { return []byte("[]"), nil })
	_, _, un1 := h.subscribe()
	_, _, un2 := h.subscribe()
	waitFor(t, func() bool { return f.count(f.started, "stats") == 1 && f.count(f.started, "events") == 1 })
	un1()
	time.Sleep(50 * time.Millisecond)
	if f.count(f.stopped, "stats") != 0 {
		t.Fatal("stopped with a subscriber left")
	}
	un2()
	un2()
	waitFor(t, func() bool { return f.count(f.stopped, "stats") == 1 && f.count(f.stopped, "events") == 1 })
	_, _, un3 := h.subscribe()
	waitFor(t, func() bool { return f.count(f.started, "stats") == 2 })
	un3()
	waitFor(t, func() bool { return f.count(f.stopped, "stats") == 2 })
}

func readEvent(t *testing.T, sc *bufio.Scanner) (string, string) {
	t.Helper()
	var name, data string
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "event: "):
			name = strings.TrimPrefix(l, "event: ")
		case strings.HasPrefix(l, "data: "):
			data = strings.TrimPrefix(l, "data: ")
		case l == "" && name != "":
			return name, data
		}
	}
	t.Fatal("stream ended")
	return "", ""
}

func TestEventsStream(t *testing.T) {
	f := newFakeStream(map[string][]string{"stats": {"\x1b[2J{\"Name\":\"ddev-a-web\",\"CPUPerc\":\"2.00%\",\"MemUsage\":\"10MiB / 1GiB\"}"}})
	h := newServerStream(t.TempDir(), &run.FakeRunner{}, &fakeActions{}, []string{t.TempDir()}, f.fn)
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)
	if name, _ := readEvent(t, sc); name != "sites" {
		t.Fatalf("first event %q", name)
	}
	name, data := readEvent(t, sc)
	if name != "stats" {
		t.Fatalf("event %q", name)
	}
	for !strings.Contains(data, `"ddev-a-web"`) {
		_, data = readEvent(t, sc)
	}
	st := decodeStats(t, []byte(data))
	if len(st.Containers) != 1 || st.Containers[0].MemBytes != 10<<20 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestEventsHostGuard(t *testing.T) {
	h := newServerStream(t.TempDir(), &run.FakeRunner{}, &fakeActions{}, nil, newFakeStream(nil).fn)
	req := httptest.NewRequest("GET", "/api/events", nil)
	req.Host = "evil.example:9900"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("code %d", rec.Code)
	}
}
