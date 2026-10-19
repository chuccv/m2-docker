package ui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"time"
)

type streamFunc func(ctx context.Context, onLine func(string), name string, args ...string) error

func execStream(ctx context.Context, onLine func(string), name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		onLine(sc.Text())
	}
	return cmd.Wait()
}

type event struct {
	name string
	data []byte
}

type timedRow struct {
	row  statRow
	seen time.Time
}

type hub struct {
	stream    streamFunc
	sites     func() ([]byte, []string)
	statsGap  time.Duration
	debounce  time.Duration
	retry     time.Duration
	stale     time.Duration
	heartbeat time.Duration

	mu        sync.Mutex
	subs      map[chan event]struct{}
	cancel    context.CancelFunc
	lastStats []byte
	lastSites []byte
	known     []string

	rowsMu sync.Mutex
	rows   map[string]timedRow
}

func newHub(stream streamFunc, sites func() ([]byte, []string)) *hub {
	return &hub{
		stream:    stream,
		sites:     sites,
		statsGap:  time.Second,
		debounce:  300 * time.Millisecond,
		retry:     2 * time.Second,
		stale:     5 * time.Second,
		heartbeat: 15 * time.Second,
		subs:      map[chan event]struct{}{},
		rows:      map[string]timedRow{},
	}
}

func (h *hub) subscribe() (chan event, []byte, func()) {
	ch := make(chan event, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if len(h.subs) == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		h.cancel = cancel
		h.rowsMu.Lock()
		h.rows = map[string]timedRow{}
		h.rowsMu.Unlock()
		go h.run(ctx)
	}
	last := h.lastStats
	h.mu.Unlock()
	return ch, last, func() { h.unsubscribe(ch) }
}

func (h *hub) unsubscribe(ch chan event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[ch]; !ok {
		return
	}
	delete(h.subs, ch)
	if len(h.subs) == 0 {
		h.cancel()
		h.cancel = nil
		h.lastStats = nil
	}
}

func (h *hub) broadcast(e event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

func (h *hub) run(ctx context.Context) {
	h.refreshSites(ctx, false)
	go h.loop(ctx, h.onStatsLine, "docker", "stats", "--format", "{{json .}}")
	changed := make(chan struct{}, 1)
	go h.loop(ctx, func(string) {
		select {
		case changed <- struct{}{}:
		default:
		}
	}, "docker", "events", "--filter", "type=container", "--filter", "event=start", "--filter", "event=die", "--filter", "event=stop", "--filter", "event=destroy", "--format", "{{json .}}")
	tick := time.NewTicker(h.statsGap)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			h.emitStats()
		case <-changed:
			select {
			case <-ctx.Done():
				return
			case <-time.After(h.debounce):
			}
			select {
			case <-changed:
			default:
			}
			h.refreshSites(ctx, true)
		}
	}
}

func (h *hub) loop(ctx context.Context, onLine func(string), name string, args ...string) {
	for ctx.Err() == nil {
		h.stream(ctx, onLine, name, args...)
		select {
		case <-ctx.Done():
		case <-time.After(h.retry):
		}
	}
}

func (h *hub) onStatsLine(l string) {
	row, ok := parseStatRow(l)
	if !ok {
		return
	}
	h.rowsMu.Lock()
	h.rows[row.Name] = timedRow{row, time.Now()}
	h.rowsMu.Unlock()
}

func (h *hub) refreshSites(ctx context.Context, publish bool) {
	data, known := h.sites()
	if ctx.Err() != nil {
		return
	}
	h.mu.Lock()
	h.known = known
	same := bytes.Equal(data, h.lastSites)
	h.lastSites = data
	h.mu.Unlock()
	if publish && data != nil && !same {
		h.broadcast(event{"sites", data})
	}
}

func (h *hub) emitStats() {
	h.mu.Lock()
	known := h.known
	h.mu.Unlock()
	var rows []statRow
	now := time.Now()
	h.rowsMu.Lock()
	for name, t := range h.rows {
		if now.Sub(t.seen) > h.stale {
			delete(h.rows, name)
			continue
		}
		rows = append(rows, t.row)
	}
	h.rowsMu.Unlock()
	data, err := json.Marshal(buildStats(rows, known))
	if err != nil {
		return
	}
	h.mu.Lock()
	h.lastStats = data
	h.mu.Unlock()
	h.broadcast(event{"stats", data})
}
