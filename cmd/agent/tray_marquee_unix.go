//go:build darwin || linux

package main

import (
	"sync"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/getlantern/systray"
)

// trayMarquee shows a static menu-bar title while remote desktop is active.
type trayMarquee struct {
	mu     sync.Mutex
	active bool
}

func (m *trayMarquee) SetActive(on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if on == m.active {
		return
	}
	m.active = on
	if !on {
		systray.SetTitle("")
		systray.SetTooltip(i18n.T("tray.tooltip_running"))
		return
	}
	systray.SetTitle(i18n.T("tray.controlled"))
	systray.SetTooltip(i18n.T("tray.tooltip_controlled"))
}
