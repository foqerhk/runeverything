//go:build windows

package main

import (
	"strings"
	"sync"
	"time"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/getlantern/systray"
)

// Windows tray has no reliable menu-bar title; bounce the tooltip instead.
type trayMarquee struct {
	mu     sync.Mutex
	stopCh chan struct{}
	active bool
}

func (m *trayMarquee) SetActive(on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if on == m.active {
		return
	}
	m.active = on
	if m.stopCh != nil {
		close(m.stopCh)
		m.stopCh = nil
	}
	if !on {
		systray.SetTitle("")
		systray.SetTooltip(i18n.T("tray.tooltip_running"))
		return
	}
	stop := make(chan struct{})
	m.stopCh = stop
	go runControlledMarqueeWin(stop)
}

func runControlledMarqueeWin(stop <-chan struct{}) {
	msg := i18n.T("tray.controlled")
	const maxPad = 6
	pad := 0
	dir := 1
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			title := strings.Repeat(" ", pad) + msg
			systray.SetTitle(title)
			systray.SetTooltip(title)
			pad += dir
			if pad >= maxPad {
				pad = maxPad
				dir = -1
			} else if pad <= 0 {
				pad = 0
				dir = 1
			}
		}
	}
}
