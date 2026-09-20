//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/winhelper"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func main() {
	if len(os.Args) < 2 {
		if isService() {
			runService()
			return
		}
		fmt.Fprintf(os.Stderr, "usage: %s <install|uninstall|start|stop|run|serve>\n", filepath.Base(os.Args[0]))
		os.Exit(2)
	}
	switch strings.ToLower(os.Args[1]) {
	case "install":
		if err := install(); err != nil {
			fatal(err)
		}
		fmt.Println("installed", winhelper.ServiceName)
	case "uninstall":
		if err := uninstall(); err != nil {
			fatal(err)
		}
		fmt.Println("uninstalled", winhelper.ServiceName)
	case "start":
		if err := startSvc(); err != nil {
			fatal(err)
		}
		fmt.Println("started", winhelper.ServiceName)
	case "stop":
		if err := stopSvc(); err != nil {
			fatal(err)
		}
		fmt.Println("stopped", winhelper.ServiceName)
	case "run", "serve":
		fmt.Println("serving", winhelper.PipeName)
		if err := winhelper.ServePipe(); err != nil {
			fatal(err)
		}
	case "service":
		runService()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func isService() bool {
	in, err := svc.IsWindowsService()
	return err == nil && in
}

func exePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

func install() error {
	exepath, err := exePath()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(winhelper.ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", winhelper.ServiceName)
	}
	s, err = m.CreateService(winhelper.ServiceName, exepath, mgr.Config{
		DisplayName: "RunEverything Secure Desktop Helper",
		Description: "LocalSystem helper for UAC/Winlogon desktop capture and input",
		StartType:   mgr.StartAutomatic,
	}, "service")
	if err != nil {
		return err
	}
	defer s.Close()
	_ = eventlog.InstallAsEventCreate(winhelper.ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err := s.Start(); err != nil {
		return err
	}
	fmt.Println("service started; waiting for session helper pipe...")
	// Session helper is spawned by the LocalSystem service (user install
	// cannot OpenProcess(winlogon)). Poll for the named pipe.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if winhelper.Available() {
			fmt.Println("helper pipe ready")
			return nil
		}
		time.Sleep(400 * time.Millisecond)
	}
	return fmt.Errorf("helper pipe not available (service running but session helper did not start)")
}

func uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(winhelper.ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()
	_, _ = s.Control(svc.Stop)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st, e := s.Query()
		if e != nil || st.State == svc.Stopped {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := s.Delete(); err != nil {
		return err
	}
	_ = eventlog.Remove(winhelper.ServiceName)
	return nil
}

func startSvc() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(winhelper.ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Start()
}

func stopSvc() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(winhelper.ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Control(svc.Stop)
	return err
}

type helperService struct{}

func (m *helperService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	exepath, err := exePath()
	if err != nil {
		exepath, _ = os.Executable()
	}
	// Session 0 cannot BitBlt the interactive UAC desktop — spawn a SYSTEM
	// helper inside the active user session (winlogon token) that owns the pipe.
	winhelper.EnsureSessionHelper(exepath)
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for c := range r {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			return false, 0
		case svc.Interrogate:
			changes <- c.CurrentStatus
		}
	}
	return false, 0
}

func runService() {
	_ = svc.Run(winhelper.ServiceName, &helperService{})
}
