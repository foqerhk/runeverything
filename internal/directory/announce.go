package directory

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
)

// Announcer periodically POSTs this relay's public URL to the directory.
type Announcer struct {
	URL      string
	Region   string
	Version  string
	Interval time.Duration
	LoadFunc func() int

	stop chan struct{}
	once sync.Once
}

func (a *Announcer) Start() {
	if a == nil || strings.TrimSpace(a.URL) == "" {
		return
	}
	if a.Interval <= 0 {
		a.Interval = DefaultAnnounceInterval
	}
	a.stop = make(chan struct{})
	go a.loop()
}

func (a *Announcer) Stop() {
	if a == nil || a.stop == nil {
		return
	}
	a.once.Do(func() { close(a.stop) })
}

func (a *Announcer) loop() {
	a.beat()
	t := time.NewTicker(a.Interval)
	defer t.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-t.C:
			a.beat()
		}
	}
}

func (a *Announcer) beat() {
	load := 0
	if a.LoadFunc != nil {
		load = a.LoadFunc()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = PostAnnounce(ctx, AnnounceRequest{
		URL:     a.URL,
		Region:  a.Region,
		Load:    load,
		Version: a.Version,
	})
}

// SharingEnabled reports whether this host should announce as a volunteer relay.
// Default true unless RE_SHARE_RELAY=0/false/off or cfg says otherwise.
func SharingEnabled(cfgShare *bool) bool {
	if v := os.Getenv("RE_SHARE_RELAY"); v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "0", "false", "no", "off":
			return false
		case "1", "true", "yes", "on":
			return true
		}
	}
	if cfgShare != nil {
		return *cfgShare
	}
	return true
}
