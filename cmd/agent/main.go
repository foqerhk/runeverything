package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/foqerhk/runeverything/internal/audit"
	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/keepalive"
	"github.com/foqerhk/runeverything/internal/netutil"
	"github.com/foqerhk/runeverything/internal/p2p"
	"github.com/foqerhk/runeverything/internal/pairing"
	"github.com/foqerhk/runeverything/internal/protocol"
	ptyx "github.com/foqerhk/runeverything/internal/pty"
	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/reudp"
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
		case "tray":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			cmdTray()
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
	fmt.Fprintf(os.Stderr, `RunEverything Agent — Intent Computing remote endpoint (RE2 only)

Usage:
  runeverything [run]     Connect to relay /re2, print pairing QR, serve PTY sessions
  runeverything tray      Windows: system tray agent (QR / autostart / quit)
  runeverything pair      Refresh pairing token and print QR (requires running agent OR local-only offer)
  runeverything status    Show device identity and config
  runeverything version   Print version

Flags (run):
  -relay URL              Relay WebSocket URL (default: auto-discover volunteer relay, or RE_RELAY)
  -public URL             URL embedded in QR for clients (defaults to -relay; path forced to /re2)
  -no-qr                  Do not print QR on start (still registers pairing token)

Volunteer relays (Bitcoin-style P2P):
  Official seeds are on GitHub (seeds.json). Agents crawl /v1/peers from seeds,
  discover more relays, and pick the lowest-ping node. Opt out of sharing with RE_SHARE_RELAY=0.
`)
}

func applyRelayDiscovery(cfg *identity.Config, relayFlag string) (discovered bool) {
	if !identity.ShouldAutoDiscover(cfg, relayFlag) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	relay, pub, ok := p2p.ResolveForAgent(ctx, cfg.RelayURL, cfg.PublicRelay, true)
	cfg.RelayURL = relay
	cfg.PublicRelay = pub
	if ok {
		log.Printf("selected relay %s (lowest ping)", relay)
		return true
	}
	return false
}

func finalizePublicRelay(cfg *identity.Config, discovered bool) {
	if cfg.PublicRelay == "" {
		cfg.PublicRelay = cfg.RelayURL
	}
	if discovered {
		return
	}
	// Colocated local default: advertise a reachable public host in the QR.
	if identity.HostIsLoopbackRelay(cfg.PublicRelay) {
		cfg.PublicRelay = netutil.ResolveClientRelay(cfg.PublicRelay, cfg.RelayURL)
	}
}

func applyRE2Paths(cfg *identity.Config) {
	cfg.RelayURL = re2.EnsurePath(cfg.RelayURL)
	if cfg.PublicRelay == "" {
		cfg.PublicRelay = cfg.RelayURL
	} else {
		cfg.PublicRelay = re2.EnsurePath(cfg.PublicRelay)
	}
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
	applyRE2Paths(cfg)
	home, _ := identity.HomeDir()
	fmt.Printf("home:         %s\n", home)
	fmt.Printf("device_id:    %s\n", id.DeviceID)
	fmt.Printf("name:         %s\n", id.Name)
	fmt.Printf("relay:        %s\n", cfg.RelayURL)
	fmt.Printf("public_relay: %s\n", cfg.PublicRelay)
	fmt.Printf("protocol:     RE2 (v%d)\n", protocol.VersionRE2)
	fmt.Printf("relay_manual: %v\n", cfg.RelayManual)
	fmt.Printf("share_relay:  %v\n", p2p.SharingEnabled(cfg.ShareRelay))
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
		cfg.RelayManual = true
	}
	if *public != "" {
		cfg.PublicRelay = *public
	}
	discovered := applyRelayDiscovery(cfg, *relay)
	finalizePublicRelay(cfg, discovered)
	applyRE2Paths(cfg)

	a := &Agent{id: id, cfg: cfg}
	noiseKP, err := identity.LoadOrCreateNoiseStatic()
	if err != nil {
		log.Fatal(err)
	}
	a.noiseKP = noiseKP
	if err := a.connectOnceRE2(); err != nil {
		log.Printf("warning: could not reach relay (%v); printing offline QR anyway", err)
		p, _, err := pairing.NewPayloadOpts(cfg.PublicRelay, id.DeviceID, id.Name, pairing.DefaultTTL, pairing.Options{
			NoisePub: identity.NoisePublicB64URL(noiseKP),
			Version:  protocol.Version,
			UDP:      udpFromRelayURL(cfg.PublicRelay),
		})
		if err != nil {
			log.Fatal(err)
		}
		_ = pairing.PrintQR(p)
		return
	}
	defer a.re2Conn.Close()
	if err := a.registerRE2(); err != nil {
		log.Fatal(err)
	}
	if err := a.offerPairRE2(true); err != nil {
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
		cfg.RelayManual = true
	}
	if identity.RelayExplicitlySet() {
		cfg.RelayManual = true
	}
	if *public != "" {
		cfg.PublicRelay = *public
	}
	discovered := applyRelayDiscovery(cfg, *relay)
	finalizePublicRelay(cfg, discovered)
	applyRE2Paths(cfg)
	_ = identity.SaveConfig(cfg)

	audit.Init()
	audit.Log("agent_start", cfg.RelayURL)

	a := &Agent{
		id:       id,
		cfg:      cfg,
		sessions: make(map[string]*ptyx.Session),
		printQR:  !*noQR,
		clip:     desktop.NewClipboardHub(),
		xferNames: make(map[string]string),
		sessionIdle: envDuration("RE_SESSION_IDLE", 30*time.Minute),
	}

	stopAwake := keepalive.Start()
	defer stopAwake()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sig
		log.Println("shutting down")
		stopAwake()
		a.closeAll()
		os.Exit(0)
	}()

	backoff := time.Second
	for {
		err := a.runLoopRE2()
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
	sessions map[string]*ptyx.Session
	mu       sync.Mutex
	printQR  bool

	re2Conn      *re2.Conn
	re2Sess      *re2.Session
	noiseKP      *re2.StaticKeyPair
	pairingToken string

	udpEP        *reudp.Endpoint
	useUDP       bool
	udpHostPort  string
	deskCancel   context.CancelFunc
	deskCap      desktop.Capturer
	deskInj      desktop.Injector
	deskEnc      desktop.Encoder
	deskSID      string
	deskFrameID  uint32
	deskABR      *desktop.ABRController
	clip         *desktop.ClipboardHub
	xferNames    map[string]string // fileID -> safe basename
	audioPlayer  *desktop.AudioPlayer
	cam          *desktop.CameraCapture
	camCancel    context.CancelFunc
	relativeMouse bool
	lastActivity time.Time
	sessionIdle  time.Duration
}

func (a *Agent) closeAll() {
	a.closeDesktop()
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, s := range a.sessions {
		_ = s.Close()
		delete(a.sessions, id)
	}
	if a.udpEP != nil {
		_ = a.udpEP.Close()
		a.udpEP = nil
	}
	if a.re2Conn != nil {
		_ = a.re2Conn.Close()
	}
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	if secs, err := time.ParseDuration(v); err == nil {
		return secs
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return def
}

func mustJSON(v interface{}) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return b
}

func (a *Agent) openSession(data re2.OpenSessionPayload) error {
	a.mu.Lock()
	if old, ok := a.sessions[data.SessionID]; ok {
		_ = old.Close()
		delete(a.sessions, data.SessionID)
	}
	a.mu.Unlock()

	cmd := data.Cmd
	useTmux := data.UseTmux
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

	go a.pumpStdoutRE2(s)
	return nil
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
