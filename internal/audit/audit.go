package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	TimeUnix int64  `json:"t"`
	Event    string `json:"event"`
	Detail   string `json:"detail,omitempty"`
}

var (
	mu   sync.Mutex
	path string
)

func Init() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".runeverything")
	_ = os.MkdirAll(dir, 0o700)
	path = filepath.Join(dir, "audit.log")
}

func Log(event, detail string) {
	mu.Lock()
	defer mu.Unlock()
	if path == "" {
		Init()
	}
	if path == "" {
		return
	}
	e := Event{TimeUnix: time.Now().Unix(), Event: event, Detail: detail}
	b, _ := json.Marshal(e)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}
