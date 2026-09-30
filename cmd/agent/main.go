package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/foqerhk/runeverything/internal/audit"
	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/i18n"
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
	i18n.Init()
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix(i18n.T("log.prefix"))

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "pair":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			cmdPair()
			return
		case "qr":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			cmdQR()
			return
		case "status":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			cmdStatus()
			return
		case "config":
			os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
			cmdConfig()
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
	fmt.Fprint(os.Stderr, i18n.T("usage"))
}

func applyRelayDiscovery(cfg *identity.Config, relayFlag string) (discovered bool) {
	if !identity.ShouldAutoDiscover(cfg, relayFlag) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Not behind NAT: stay on local relay and advertise our own public IP.
	// Do not pick a volunteer middle hop.
	if ip, ok := netutil.DirectPublicIP(ctx); ok {
		if cfg.RelayURL == "" || identity.HostIsLoopbackRelay(cfg.RelayURL) {
			if cfg.RelayURL == "" {
				cfg.RelayURL = "ws://127.0.0.1:8787/re2"
			}
			cfg.PublicRelay = netutil.AdvertiseRelayURL(cfg.RelayURL, ip)
			i18n.Log("log.direct_public", ip)
		}
		return false
	}

	i18n.Log("log.behind_nat")
	relay, pub, ok := p2p.ResolveForAgent(ctx, cfg.RelayURL, cfg.PublicRelay, true)
	cfg.RelayURL = relay
	cfg.PublicRelay = pub
	if ok {
		i18n.Log("log.selected_relay", relay)
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
	applyNetworkPrefs(cfg)
	applyRE2Paths(cfg)
	home, _ := identity.HomeDir()
	fmt.Print(i18n.T("status.home", home))
	fmt.Print(i18n.T("status.device_id", id.DeviceID))
	fmt.Print(i18n.T("status.name", id.Name))
	fmt.Print(i18n.T("status.relay", cfg.RelayURL))
	fmt.Print(i18n.T("status.public_relay", cfg.PublicRelay))
	fmt.Print(i18n.T("status.protocol", protocol.VersionRE2))
	fmt.Print(i18n.T("status.relay_manual", cfg.RelayManual))
	fmt.Print(i18n.T("status.share_relay", p2p.SharingEnabled(cfg.ShareRelay)))
	osName, arch := identity.PlatformInfo()
	fmt.Print(i18n.T("status.platform", osName, arch))
	fmt.Print(i18n.T("status.lang", i18n.Active()))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if ip, ok := netutil.DirectPublicIP(ctx); ok {
		fmt.Print(i18n.T("status.nat_no", ip))
	} else {
		fmt.Print(i18n.T("status.nat_yes"))
	}
	printHostPermissionStatus()
}

func printHostPermissionStatus() {
	if runtime.GOOS != "darwin" {
		return
	}
	p := desktop.CheckHostPermissions()
	yes, no := i18n.T("status.perm_yes"), i18n.T("status.perm_no")
	if p.ScreenRecording {
		fmt.Print(i18n.T("status.perm_screen", yes))
	} else {
		fmt.Print(i18n.T("status.perm_screen", no))
	}
	if p.Accessibility {
		fmt.Print(i18n.T("status.perm_ax", yes))
	} else {
		fmt.Print(i18n.T("status.perm_ax", no))
	}
}

func logHostPermissions(p desktop.HostPermissions) {
	if runtime.GOOS != "darwin" {
		return
	}
	if p.ScreenRecording && p.Accessibility {
		i18n.Log("log.perm_ok")
		return
	}
	if !p.ScreenRecording {
		i18n.Log("log.perm_screen_need")
	}
	if !p.Accessibility {
		i18n.Log("log.perm_ax_need")
	}
	i18n.Log("log.perm_mic_hint")
	i18n.Log("log.perm_camera_hint")
}

// cmdQR prints the current pairing QR from ~/.runeverything/last_pairing.json
// without opening a second relay connection (safe while `run` is already up).
func cmdQR() {
	home, err := identity.HomeDir()
	if err != nil {
		log.Fatal(err)
	}
	path := filepath.Join(home, "last_pairing.json")
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(i18n.T("qr.missing", path))
	}
	var p protocol.PairingPayload
	if err := json.Unmarshal(b, &p); err != nil {
		log.Fatal(err)
	}
	if p.PairingToken == "" || p.DeviceID == "" || p.Relay == "" {
		log.Fatal(i18n.T("qr.incomplete", path))
	}
	if p.ExpiresAt > 0 && p.ExpiresAt <= time.Now().Unix() {
		log.Fatal(i18n.T("qr.expired", time.Unix(p.ExpiresAt, 0).Format(time.RFC3339)))
	}
	left := time.Until(time.Unix(p.ExpiresAt, 0)).Round(time.Second)
	fmt.Println(i18n.T("qr.ok", path, left))
	if err := pairing.PrintQR(&p); err != nil {
		log.Fatal(err)
	}
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
	applyNetworkPrefs(cfg)
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
		i18n.Log("log.relay_offline", err)
		log.Fatal("cannot mint a usable pairing QR while relay is offline (token would not be redeemable)")
	}
	defer a.re2Conn.Close()
	if err := a.registerRE2(); err != nil {
		log.Fatal(err)
	}
	if err := a.offerPairRE2(true, true); err != nil {
		log.Fatal(err)
	}
}

func cmdRun() {
	relay := flag.String("relay", "", "relay WebSocket URL")
	public := flag.String("public", "", "public relay URL embedded in QR")
	noQR := flag.Bool("no-qr", false, "do not print QR on start")
	flag.Parse()

	release, err := acquireAgentLock()
	if err != nil {
		log.Fatal(err)
	}
	defer release()

	id, err := identity.LoadOrCreate()
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := identity.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	applyNetworkPrefs(cfg)
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

	// macOS: request Screen Recording + Accessibility up front so remote desktop works.
	logHostPermissions(desktop.EnsureHostPermissions())

	stopAwake := keepalive.Start()
	defer stopAwake()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sig
		i18n.Log("log.shutting_down")
		stopAwake()
		a.closeAll()
		os.Exit(0)
	}()

	backoff := time.Second
	for {
		err := a.runLoopRE2()
		if err != nil {
			i18n.Log("log.disconnected", err, backoff)
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
	pairingToken      string
	pairingExpiresAt  int64 // unix seconds; reused across reconnects until expired

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
		i18n.Log("log.session_closed", id, reason)
	}
}
