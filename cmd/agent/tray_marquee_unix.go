//go:build darwin || linux

package main

import (
	"strings"
	"sync"
	"time"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/getlantern/systray"
)

// trayMarquee bounces the "being controlled" title beside the menu-bar icon.
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
	systray.SetTooltip(i18n.T("tray.tooltip_controlled"))
	go runControlledMarquee(stop)
}

func runControlledMarquee(stop <-chan struct{}) {
	msg := i18n.T("tray.controlled")
	if msg == "" {
		msg = "被控制中..."
	}
	// Bounce leading spaces so the phrase walks left ↔ right in the menu bar.
	const maxPad = 8
	pad := 0
	dir := 1
	tick := time.NewTicker(180 * time.Millisecond)
	defer tick.Stop()

	render := func() {
		systray.SetTitle(strings.Repeat(" ", pad) + msg)
	}
	render()

	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			pad += dir
			if pad >= maxPad {
				pad = maxPad
				dir = -1
			} else if pad <= 0 {
				pad = 0
				dir = 1
			}
			render()
		}
	}
}
