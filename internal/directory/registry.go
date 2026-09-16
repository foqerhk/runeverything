package directory

import (
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultAnnounceInterval = 30 * time.Second
	DefaultTTL              = 90 * time.Second
	// DefaultDirectoryURL is the bootstrap discovery endpoint.
	DefaultDirectoryURL = "https://foqerhk.github.io/runeverything/relays.json"
	DefaultAnnounceURL  = "" // set via RE_DIRECTORY_ANNOUNCE when running a live directory
)

// Entry is one volunteer relay in the public directory.
type Entry struct {
	URL      string    `json:"url"`
	Region   string    `json:"region,omitempty"`
	Load     int       `json:"load"`
	Version  string    `json:"version,omitempty"`
	LastSeen time.Time `json:"last_seen"`
}

// AnnounceRequest is POSTed by volunteer relays.
type AnnounceRequest struct {
	URL     string `json:"url"`
	Region  string `json:"region,omitempty"`
	Load    int    `json:"load"`
	Version string `json:"version,omitempty"`
}

// ListResponse is returned by GET /v1/relays (and mirrors relays.json shape).
type ListResponse struct {
	Relays []Entry `json:"relays"`
}

// Registry is an in-memory TTL map of volunteer relays.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*Entry // key = normalized url
	ttl     time.Duration
	allowWS bool
}

func NewRegistry(ttl time.Duration, allowWS bool) *Registry {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Registry{
		entries: make(map[string]*Entry),
		ttl:     ttl,
		allowWS: allowWS,
	}
}

func NormalizeRelayURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "wss" && u.Scheme != "ws" {
		return "", errInvalidScheme
	}
	if u.Host == "" {
		return "", errInvalidHost
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/ws"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

const (
	errInvalidScheme simpleError = "url scheme must be wss (or ws in dev)"
	errInvalidHost   simpleError = "url host required"
)

func (r *Registry) Announce(req AnnounceRequest) (*Entry, error) {
	u, err := NormalizeRelayURL(req.URL)
	if err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(u)
	if parsed.Scheme == "ws" && !r.allowWS {
		return nil, errInvalidScheme
	}
	if req.Load < 0 {
		req.Load = 0
	}
	now := time.Now().UTC()
	ent := &Entry{
		URL:      u,
		Region:   strings.TrimSpace(req.Region),
		Load:     req.Load,
		Version:  strings.TrimSpace(req.Version),
		LastSeen: now,
	}
	r.mu.Lock()
	r.entries[u] = ent
	r.mu.Unlock()
	cp := *ent
	return &cp, nil
}

func (r *Registry) List() []Entry {
	r.expire()
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		cp := *e
		out = append(out, cp)
	}
	return out
}

func (r *Registry) expire() {
	cutoff := time.Now().UTC().Add(-r.ttl)
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, e := range r.entries {
		if e.LastSeen.Before(cutoff) {
			delete(r.entries, k)
		}
	}
}
