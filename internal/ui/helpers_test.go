package ui

import (
	"encoding/json"
	"testing"
)

func decodeSites(t *testing.T, b []byte) map[string]SiteView {
	t.Helper()
	var l []SiteView
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatal(err)
	}
	m := map[string]SiteView{}
	for _, s := range l {
		m[s.Name] = s
	}
	return m
}

type statsResp struct {
	Sites      []Usage `json:"sites"`
	Shared     Usage   `json:"shared"`
	Containers []Usage `json:"containers"`
}

func decodeStats(t *testing.T, b []byte) statsResp {
	t.Helper()
	var s statsResp
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}
