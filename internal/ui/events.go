package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *server) sitesPayload() ([]byte, []string) {
	views, err := s.siteViews()
	if err != nil {
		return nil, nil
	}
	data, err := json.Marshal(views)
	if err != nil {
		return nil, nil
	}
	known := make([]string, 0, len(views))
	for _, v := range views {
		known = append(known, v.Name)
	}
	return data, known
}

func writeEvent(w http.ResponseWriter, f http.Flusher, e event) error {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, e.data); err != nil {
		return err
	}
	f.Flush()
	return nil
}

func (s *server) handleEvents(w http.ResponseWriter, req *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, last, unsubscribe := s.hub.subscribe()
	defer unsubscribe()
	if data, _ := s.sitesPayload(); data != nil {
		if writeEvent(w, f, event{"sites", data}) != nil {
			return
		}
	}
	if last != nil {
		if writeEvent(w, f, event{"stats", last}) != nil {
			return
		}
	}
	f.Flush()
	beat := time.NewTicker(s.hub.heartbeat)
	defer beat.Stop()
	for {
		select {
		case <-req.Context().Done():
			return
		case e := <-ch:
			if writeEvent(w, f, e) != nil {
				return
			}
		case <-beat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			f.Flush()
		}
	}
}
