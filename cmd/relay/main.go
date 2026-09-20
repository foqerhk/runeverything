package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/foqerhk/runeverything/internal/auth"
	"github.com/foqerhk/runeverything/internal/bandwidth"
	"github.com/foqerhk/runeverything/internal/netutil"
	"github.com/foqerhk/runeverything/internal/p2p"
	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/registrar"
)

type deviceState struct {
	ID     string
	Secret string
	Name   string
	// RE2 peers (binary outer frames on /re2).
	RE2Agent  *re2.Conn
	RE2Client *re2.Conn
	// REUDP peers (datagram data plane).
	UDPAgent  *net.UDPAddr
	UDPClient *net.UDPAddr
}

type Hub struct {
	mu        sync.RWMutex
	devices   map[string]*deviceState
	auth      *auth.Store
	public    string // advertised relay URL (/re2)
	publicUDP string // host:port for REUDP
	limiter   *bandwidth.Limiter
}

func NewHub(public string) *Hub {
	est := bandwidth.LoadFromEnv()
	// Prefer measured file from probe-bandwidth.sh
	if home, err := os.UserHomeDir(); err == nil {
		if b, err := os.ReadFile(home + "/.runeverything/bandwidth.json"); err == nil {
			var m struct {
				MbpsUp      float64 `json:"mbps_up"`
				MaxSessions int     `json:"max_sessions"`
				PerUserKbps int     `json:"per_user_kbps"`
			}
			if json.Unmarshal(b, &m) == nil && m.MaxSessions > 0 {
				est.MbpsUp = m.MbpsUp
				est.MaxSessions = m.MaxSessions
				if m.PerUserKbps > 0 {
					est.PerUserKbps = m.PerUserKbps
				}
			}
		}
	}
	log.Printf("relay capacity: max_sessions=%d (%.1f Mbps up, %d kbps/user)", est.MaxSessions, est.MbpsUp, est.PerUserKbps)
	return &Hub{
		devices: make(map[string]*deviceState),
		auth:    auth.NewStore(),
		public:  public,
		limiter: bandwidth.NewLimiter(est),
	}
}

func (h *Hub) connectionLoad() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, d := range h.devices {
		if d.RE2Agent != nil {
			n++
		}
		if d.RE2Client != nil {
			n++
		}
	}
	return n
}

func main() {
	listen := flag.String("listen", ":8787", "HTTP listen address (ignored when RE_AUTO_DOMAIN=1; then :80/:443)")
	udpListen := flag.String("udp", envOr("RE_UDP_LISTEN", ""), "UDP listen address for REUDP (default: same host port as -listen)")
	public := flag.String("public", "", "Public relay WebSocket URL advertised to clients (default derived from public IP, path /re2)")
	publicUDP := flag.String("public-udp", envOr("RE_PUBLIC_UDP", ""), "Public UDP host:port advertised in BIND_OK / QR (default derived)")
	region := flag.String("region", os.Getenv("RE_REGION"), "optional region tag for peer gossip")
	share := flag.Bool("share", true, "advertise this relay via P2P gossip (override with RE_SHARE_RELAY=0)")
	allowWS := flag.Bool("allow-ws", false, "accept ws:// peers in gossip (dev only; path still /re2)")
	autoDomain := flag.Bool("auto-domain", registrar.AutoClaimEnabled(), "claim official subdomain + ACME wss (RE_AUTO_DOMAIN=1)")
	flag.Parse()

	hub := NewHub(*public)

	shareCfg := *share
	sharing := p2p.SharingEnabled(&shareCfg)

	publicURL := strings.TrimSpace(*public)
	hostname := ""
	if *autoDomain {
		if !sharing {
			log.Fatal("auto-domain requires sharing enabled (unset RE_SHARE_RELAY=0); join token is issued when sharing")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		wssURL, host, err := registrar.ClaimPublicWSS(ctx)
		cancel()
		if err != nil {
			log.Fatalf("auto-domain claim failed: %v", err)
		}
		hostname = host
		if publicURL == "" {
			publicURL = wssURL
		}
	}
	if publicURL == "" {
		publicURL = derivePublicRelayURL(*listen)
	}
	if publicURL != "" {
		publicURL = re2.EnsurePath(publicURL)
	}
	hub.public = publicURL

	udpAddr := strings.TrimSpace(*udpListen)
	if udpAddr == "" {
		udpAddr = deriveUDPListen(*listen)
	}
	pubUDP := strings.TrimSpace(*publicUDP)
	if pubUDP == "" {
		pubUDP = derivePublicUDP(publicURL, udpAddr)
	}
	hub.publicUDP = pubUDP

	store := p2p.NewStore(p2p.DefaultPeerTTL, *allowWS || strings.HasPrefix(publicURL, "ws://"))
	if publicURL != "" {
		store.SetSelf(publicURL)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	p2p.Mount(mux, store, sharing && publicURL != "" && !netutil.IsLoopbackHost(hostOfURL(publicURL)))
	bandwidth.MountCapacity(mux, hub.limiter)
	mux.HandleFunc("/re2", func(w http.ResponseWriter, r *http.Request) {
		handleRE2(hub, w, r)
	})

	if sharing && publicURL != "" && !netutil.IsLoopbackHost(hostOfURL(publicURL)) {
		g := &p2p.Gossiper{
			Store:   store,
			Share:   true,
			Region:  *region,
			Version: "0.1.0",
			LoadFunc: func() int {
				return hub.limiter.BusyScore()
			},
		}
		g.Start()
		log.Printf("p2p share enabled; self=%s", publicURL)
	} else {
		log.Printf("p2p share disabled (or no public URL); peer exchange still serves known addrs")
	}

	uaddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		log.Fatalf("udp resolve: %v", err)
	}
	uconn, err := net.ListenUDP("udp", uaddr)
	if err != nil {
		log.Fatalf("udp listen: %v", err)
	}
	go serveUDP(hub, uconn, pubUDP)

	if hostname != "" {
		log.Printf("RunEverything relay auto-tls hostname=%s public=%s udp=%s", hostname, publicURL, pubUDP)
		if err := registrar.ServeAutoTLS(hostname, mux); err != nil {
			log.Fatal(err)
		}
		return
	}

	log.Printf("RunEverything relay listening on %s (path /re2) udp=%s public=%s public_udp=%s", *listen, udpAddr, publicURL, pubUDP)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func deriveUDPListen(listen string) string {
	if strings.HasPrefix(listen, ":") {
		return listen
	}
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		return ":8787"
	}
	return ":" + port
}

func derivePublicUDP(publicURL, udpListen string) string {
	port := "8787"
	if strings.HasPrefix(udpListen, ":") {
		port = strings.TrimPrefix(udpListen, ":")
	} else if _, p, err := net.SplitHostPort(udpListen); err == nil && p != "" {
		port = p
	}
	host := ""
	if publicURL != "" {
		if u, err := url.Parse(publicURL); err == nil {
			host = u.Hostname()
		}
	}
	if host == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		host = netutil.DetectPublicHost(ctx)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func hostOfURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Hostname()
}

func derivePublicRelayURL(listen string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	host := netutil.DetectPublicHost(ctx)
	if host == "" {
		return ""
	}
	port := "8787"
	if strings.HasPrefix(listen, ":") {
		port = strings.TrimPrefix(listen, ":")
	} else if _, p, err := net.SplitHostPort(listen); err == nil && p != "" {
		port = p
	}
	scheme := "ws"
	if os.Getenv("RE_RELAY_TLS") == "1" {
		scheme = "wss"
	}
	return fmt.Sprintf("%s://%s/re2", scheme, net.JoinHostPort(host, port))
}

func publicRelay(hub *Hub, r *http.Request) string {
	if hub.public != "" {
		return hub.public
	}
	scheme := "ws"
	if r.TLS != nil {
		scheme = "wss"
	}
	// X-Forwarded-Proto support for reverse proxies.
	if xp := r.Header.Get("X-Forwarded-Proto"); xp == "https" {
		scheme = "wss"
	} else if xp == "http" {
		scheme = "ws"
	}
	return scheme + "://" + r.Host + "/re2"
}
