package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/pairing"
	"github.com/foqerhk/runeverything/internal/protocol"
	ptyx "github.com/foqerhk/runeverything/internal/pty"
	"github.com/foqerhk/runeverything/internal/tunnel"
)

// Set via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("runeverything: ")

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "pair":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			cmdPair()
			return
		case "status":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			cmdStatus()
			return
		case "run":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			cmdRun()
			return
		case "version", "-v", "--version":
			fmt.Println(version)
			return
		case "help", "-h", "--help":
			printUsage()
			return
		}
	}
	cmdRun()
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `RunEverything Agent — Intent Computing remote endpoint

Usage:
  runeverything [run]     Connect to relay, print pairing QR, serve PTY sessions
  runeverything pair      Refresh pairing token and print QR (requires running agent OR local-only offer)
  runeverything status    Show device identity and config
  runeverything version   Print version

Flags (run):
  -relay URL              Relay WebSocket URL (default from config / RE_RELAY)
  -public URL             URL embedded in QR for clients (defaults to -relay)
  -no-qr                  Do not print QR on start (still registers pairing token)
`)
}

func cmdStatus() {
	id, err := identity.LoadOrCreate()
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := identity.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	home, _ := identity.HomeDir()
	fmt.Printf("home:         %s\n", home)
	fmt.Printf("device_id:    %s\n", id.DeviceID)
	fmt.Printf("name:         %s\n", id.Name)
	fmt.Printf("relay:        %s\n", cfg.RelayURL)
	fmt.Printf("public_relay: %s\n", cfg.PublicRelay)
	osName, arch := identity.PlatformInfo()
	fmt.Printf("platform:     %s/%s\n", osName, arch)
}

func cmdPair() {
	relay := flag.String("relay", "", "relay URL")
	public := flag.String("public", "", "public relay URL for QR")
	flag.Parse()

	id, err := identity.LoadOrCreate()
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := identity.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if *relay != "" {
		cfg.RelayURL = *relay
	}
	if *public != "" {
		cfg.PublicRelay = *public
	}
	if cfg.PublicRelay == "" {
		cfg.PublicRelay = cfg.RelayURL
	}

	// Connect briefly to publish pair_offer, then print QR.
	a := &Agent{id: id, cfg: cfg}
	if err := a.connectOnce(); err != nil {
		log.Printf("warning: could not reach relay (%v); printing offline QR anyway", err)
		p, _, err := pairing.NewPayload(cfg.PublicRelay, id.DeviceID, id.Name, pairing.DefaultTTL)
		if err != nil {
			log.Fatal(err)
		}
		_ = pairing.PrintQR(p)
		return
	}
	defer a.conn.Close()
	if err := a.register(); err != nil {
		log.Fatal(err)
	}
	if err := a.offerPair(true); err != nil {
		log.Fatal(err)
	}
}

func cmdRun() {
	relay := flag.String("relay", "", "relay WebSocket URL")
	public := flag.String("public", "", "public relay URL embedded in QR")
	noQR := flag.Bool("no-qr", false, "do not print QR on start")
	flag.Parse()

	id, err := identity.LoadOrCreate()
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := identity.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if *relay != "" {
		cfg.RelayURL = *relay
	}
	if *public != "" {
		cfg.PublicRelay = *public
	}
	if cfg.PublicRelay == "" {
		cfg.PublicRelay = cfg.RelayURL
	}
	_ = identity.SaveConfig(cfg)

	a := &Agent{
		id:       id,
		cfg:      cfg,
		sessions: make(map[string]*ptyx.Session),
		printQR:  !*noQR,
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sig
		log.Println("shutting down")
		a.closeAll()
		os.Exit(0)
	}()

	backoff := time.Second
	for {
		err := a.runLoop()
		if err != nil {
			log.Printf("disconnected: %v; reconnecting in %s", err, backoff)
		}
		time.Sleep(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

type Agent struct {
	id       *identity.Identity
	cfg      *identity.Config
	conn     *tunnel.Conn
	sessions map[string]*ptyx.Session
	mu       sync.Mutex
	printQR  bool
}

func (a *Agent) closeAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, s := range a.sessions {
		_ = s.Close()
		delete(a.sessions, id)
	}
	if a.conn != nil {
		_ = a.conn.Close()
	}
}

func (a *Agent) connectOnce() error {
	u, err := url.Parse(a.cfg.RelayURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("role", protocol.RoleAgent)
	u.RawQuery = q.Encode()
	c, _, err := tunnel.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	a.conn = c
	return nil
}

func (a *Agent) register() error {
	osName, arch := identity.PlatformInfo()
	b, err := protocol.Encode(protocol.TypeRegister, "reg1", protocol.RegisterData{
		DeviceID:     a.id.DeviceID,
		DeviceSecret: a.id.DeviceSecret,
		Name:         a.id.Name,
		OS:           osName,
		Arch:         arch,
	})
	if err != nil {
		return err
	}
	if err := a.conn.WriteText(b); err != nil {
		return err
	}
	_ = a.conn.WS.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, msg, err := a.conn.WS.ReadMessage()
	if err != nil {
		return err
	}
	env, err := protocol.Decode(msg)
	if err != nil {
		return err
	}
	if env.Type == protocol.TypeError {
		var ed protocol.ErrorData
		_ = protocol.DecodeData(env, &ed)
		return fmt.Errorf("register failed: %s", ed.Message)
	}
	if env.Type != protocol.TypeRegisterOK {
		return fmt.Errorf("unexpected register response: %s", env.Type)
	}
	_ = a.conn.WS.SetReadDeadline(time.Time{})
	return nil
}

func (a *Agent) offerPair(print bool) error {
	p, token, err := pairing.NewPayload(a.cfg.PublicRelay, a.id.DeviceID, a.id.Name, pairing.DefaultTTL)
	if err != nil {
		return err
	}
	b, err := protocol.Encode(protocol.TypePairOffer, "pair1", protocol.PairOfferData{
		DeviceID:     a.id.DeviceID,
		PairingToken: token,
		Name:         a.id.Name,
		ExpiresAt:    p.ExpiresAt,
		Relay:        a.cfg.PublicRelay,
	})
	if err != nil {
		return err
	}
	if err := a.conn.WriteText(b); err != nil {
		return err
	}
	// Persist last pairing for status/debug.
	home, _ := identity.EnsureHome()
	_ = os.WriteFile(filepath.Join(home, "last_pairing.json"), mustJSON(p), 0o600)
	if print || a.printQR {
		return pairing.PrintQR(p)
	}
	return nil
}

func mustJSON(v interface{}) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return b
}

func (a *Agent) runLoop() error {
	if err := a.connectOnce(); err != nil {
		return err
	}
	defer a.conn.Close()

	if err := a.register(); err != nil {
		return err
	}
	log.Printf("registered as %s (%s) via %s", a.id.Name, a.id.DeviceID, a.cfg.RelayURL)

	if err := a.offerPair(a.printQR); err != nil {
		log.Printf("pair offer: %v", err)
	}
	a.printQR = false // only first connect in this process prints unless pair cmd

	done := make(chan struct{})
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				b, _ := protocol.Encode(protocol.TypePing, "", nil)
				_ = a.conn.WriteText(b)
			}
		}
	}()
	defer close(done)

	for {
		mt, msg, err := a.conn.WS.ReadMessage()
		if err != nil {
			return err
		}
		if mt == websocket.BinaryMessage {
			dir, sid, payload, err := protocol.DecodePTYFrame(msg)
			if err != nil {
				continue
			}
			if dir != protocol.DirStdin {
				continue
			}
			a.mu.Lock()
			s := a.sessions[sid]
			a.mu.Unlock()
			if s != nil {
				_, _ = s.Write(payload)
			}
			continue
		}

		env, err := protocol.Decode(msg)
		if err != nil {
			continue
		}
		switch env.Type {
		case protocol.TypeSessionOpen:
			var data protocol.SessionOpenData
			if err := protocol.DecodeData(env, &data); err != nil {
				a.sendErr(env.ID, "bad_data", err.Error())
				continue
			}
			if err := a.openSession(data); err != nil {
				a.sendErr(env.ID, "session_open_failed", err.Error())
				continue
			}
			b, _ := protocol.Encode(protocol.TypeSessionReady, env.ID, protocol.SessionReadyData{SessionID: data.SessionID})
			_ = a.conn.WriteText(b)

		case protocol.TypeSessionClose:
			var data protocol.SessionCloseData
			_ = protocol.DecodeData(env, &data)
			a.closeSession(data.SessionID, data.Reason)

		case protocol.TypeResize:
			var data protocol.ResizeData
			_ = protocol.DecodeData(env, &data)
			a.mu.Lock()
			s := a.sessions[data.SessionID]
			a.mu.Unlock()
			if s != nil {
				_ = s.Resize(data.Cols, data.Rows)
			}

		case protocol.TypePairAck, protocol.TypePong, protocol.TypeRegisterOK:
			// ignore
		case protocol.TypeError:
			var ed protocol.ErrorData
			_ = protocol.DecodeData(env, &ed)
			log.Printf("relay error: %s %s", ed.Code, ed.Message)
		default:
			log.Printf("unknown control: %s", env.Type)
		}
	}
}

func (a *Agent) sendErr(id, code, msg string) {
	b, _ := protocol.Encode(protocol.TypeError, id, protocol.ErrorData{Code: code, Message: msg})
	_ = a.conn.WriteText(b)
}

func (a *Agent) openSession(data protocol.SessionOpenData) error {
	a.mu.Lock()
	if old, ok := a.sessions[data.SessionID]; ok {
		_ = old.Close()
		delete(a.sessions, data.SessionID)
	}
	a.mu.Unlock()

	cmd := data.Cmd
	useTmux := data.UseTmux
	// If cmd looks like wanting coding agent and empty, leave to client.
	if len(cmd) == 1 && strings.TrimSpace(cmd[0]) == "" {
		cmd = nil
	}

	s, err := ptyx.Start(data.SessionID, data.Cwd, cmd, data.Cols, data.Rows, useTmux, data.TmuxName)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.sessions[data.SessionID] = s
	a.mu.Unlock()

	go a.pumpStdout(s)
	return nil
}

func (a *Agent) pumpStdout(s *ptyx.Session) {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			frame := protocol.EncodePTYFrame(protocol.DirStdout, s.ID, buf[:n])
			if werr := a.conn.WriteBinary(frame); werr != nil {
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("session %s read: %v", s.ID, err)
			}
			a.closeSession(s.ID, "pty_exit")
			b, _ := protocol.Encode(protocol.TypeSessionClose, "", protocol.SessionCloseData{
				SessionID: s.ID,
				Reason:    "pty_exit",
			})
			_ = a.conn.WriteText(b)
			return
		}
	}
}

func (a *Agent) closeSession(id, reason string) {
	a.mu.Lock()
	s, ok := a.sessions[id]
	if ok {
		delete(a.sessions, id)
	}
	a.mu.Unlock()
	if ok {
		_ = s.Close()
		log.Printf("session closed %s (%s)", id, reason)
	}
}
