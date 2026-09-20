//go:build !windows

package deskbridge

import "github.com/foqerhk/runeverything/internal/desktop"

func NewCapturer() (desktop.Capturer, error) { return desktop.NewCapturer() }
func NewInjector() (desktop.Injector, error) { return desktop.NewInjector() }
