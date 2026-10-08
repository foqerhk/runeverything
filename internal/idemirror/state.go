package idemirror

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/url"
	"strings"
)

// chatState is the structured view KoKo renders above the terminal.
type chatState struct {
	V              int          `json:"v"`
	Kind           string       `json:"kind"`
	ComposerID     string       `json:"composerId"`
	Name           string       `json:"name"`
	Cwd            string       `json:"cwd"`
	Mode           string       `json:"mode"`
	Modes          []option     `json:"modes"`
	Model          string       `json:"model"`
	Models         []option     `json:"models"`
	ContextPercent float64      `json:"contextPercent"`
	LinesAdded     int          `json:"linesAdded"`
	LinesRemoved   int          `json:"linesRemoved"`
	Files          []stateFile  `json:"files"`
	Bridge         bridgeStatus `json:"bridge"`
}

type stateFile struct {
	Path string `json:"path"`
	New  bool   `json:"new"`
}

type bridgeStatus struct {
	Ready       bool `json:"ready"`
	NeedsReload bool `json:"needsReload"`
}

// publishState writes chat state as an OSC sequence when it changed.
func (m *mirror) publishState() {
	m.mu.Lock()
	meta := m.meta
	m.mu.Unlock()
	st := chatState{
		V: 1, Kind: "cursor-ide", ComposerID: m.opt.ComposerID,
		Name: meta.Name, Cwd: meta.Cwd, Mode: meta.Mode, Modes: m.modes,
		Model: meta.Model, Models: m.models, ContextPercent: meta.ContextPercent,
		LinesAdded: meta.LinesAdded, LinesRemoved: meta.LinesRemoved,
		Files: stateFiles(meta),
	}
	if r, ok := pickBridge(bridges(m.home), meta.Cwd); ok {
		st.Bridge.Ready = bridgeSupports(r)
		st.Bridge.NeedsReload = !st.Bridge.Ready
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if string(raw) == m.lastState {
		return
	}
	m.lastState = string(raw)
	_, _ = io.WriteString(m.out, oscStateTag+base64.StdEncoding.EncodeToString(raw)+"\x07")
}

func stateFiles(meta composerMeta) []stateFile {
	seen := map[string]bool{}
	var out []stateFile
	add := func(uri string, isNew bool) {
		p := uriPath(uri)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, stateFile{Path: p, New: isNew})
	}
	for _, f := range meta.Files {
		add(f.URI, f.New)
	}
	for _, u := range meta.CreatedFiles {
		add(u, true)
	}
	return out
}

func uriPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}
	u, err := url.Parse(uri)
	if err != nil {
		return strings.TrimPrefix(uri, "file://")
	}
	return u.Path
}
