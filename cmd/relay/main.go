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

	"github.com/gorilla/websocket"
	"github.com/foqerhk/runeverything/internal/auth"
	"github.com/foqerhk/runeverything/internal/netutil"
	"github.com/foqerhk/runeverything/internal/p2p"
	"github.com/foqerhk/runeverything/internal/protocol"
	"github.com/foqerhk/runeverything/internal/tunnel"
)

type deviceState struct {
	ID           string
	Secret       string
	Name         string
	Agent        *tunnel.Conn
	Clients      map[*tunnel.Conn]struct{}
	SessionRoute map[string]*tunnel.Conn // session_id -> client
}

type Hub struct {
	mu      sync.RWMutex
	devices map[string]*deviceState
	auth    *auth.Store
	public  string // advertised relay URL
}

func NewHub(public string) *Hub {
	return &Hub{
		devices: make(map[string]*deviceState),
		auth:    auth.NewStore(),
		public:  public,
	}
}

func (h *Hub) getOrCreate(id string) *deviceState {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		d = &deviceState{
			ID:           id,
			Clients:      make(map[*tunnel.Conn]struct{}),
			SessionRoute: make(map[string]*tunnel.Conn),
		}
		h.devices[id] = d
	}
	return d
}

func (h *Hub) setAgent(id, secret, name string, c *tunnel.Conn) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		d = &deviceState{
			ID:           id,
			Clients:      make(map[*tunnel.Conn]struct{}),
			SessionRoute: make(map[string]*tunnel.Conn),
		}
		h.devices[id] = d
	}
	if d.Secret != "" && !auth.ConstantTimeEqual(d.Secret, secret) {
		return fmt.Errorf("invalid device secret")
	}
	if d.Agent != nil {
		_ = d.Agent.Close()
	}
	d.Secret = secret
	d.Name = name
	d.Agent = c
	return nil
}

func (h *Hub) clearAgent(id string, c *tunnel.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		return
	}
	if d.Agent == c {
		d.Agent = nil
	}
}

func (h *Hub) agentOf(id string) *tunnel.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	d := h.devices[id]
	if d == nil {
		return nil
	}
	return d.Agent
}

func (h *Hub) addClient(id string, c *tunnel.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d := h.devices[id]
	if d == nil {
		d = &deviceState{
			ID:           id,
			Clients:      make(map[*tunnel.Conn]struct{}),
			SessionRoute: make(map[string]*tunnel.Conn),
		}
		h.devices[id] = d
	}
	d.Clients[c] = struct{}{}
}

func (h *Hub) removeClient(id string, c *tunnel.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d := h.devices[id]
	if d == nil {
		return
	}
	delete(d.Clients, c)
	for sid, cli := range d.SessionRoute {
		if cli == c {
			delete(d.SessionRoute, sid)
		}
	}
}

func (h *Hub) bindSession(deviceID, sessionID string, c *tunnel.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d := h.devices[deviceID]
	if d == nil {
		return
	}
	d.SessionRoute[sessionID] = c
}

func (h *Hub) clientForSession(deviceID, sessionID string) *tunnel.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	d := h.devices[deviceID]
	if d == nil {
		return nil
	}
	return d.SessionRoute[sessionID]
}

func (h *Hub) unbindSession(deviceID, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	d := h.devices[deviceID]
	if d == nil {
		return
	}
	delete(d.SessionRoute, sessionID)
}

func (h *Hub) deviceOnline(id string) (name string, online bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	d := h.devices[id]
	if d == nil {
		return "", false
	}
	return d.Name, d.Agent != nil
}

func (h *Hub) connectionLoad() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, d := range h.devices {
		if d.Agent != nil {
			n++
		}
		n += len(d.Clients)
	}
	return n
}

func writeErr(c *tunnel.Conn, id, code, msg string) {
	b, _ := protocol.Encode(protocol.TypeError, id, protocol.ErrorData{Code: code, Message: msg})
	_ = c.WriteText(b)
}

func main() {
	listen := flag.String("listen", ":8787", "HTTP listen address")
	public := flag.String("public", "", "Public relay WebSocket URL advertised to clients (default derived from public IP)")
	region := flag.String("region", os.Getenv("RE_REGION"), "optional region tag for peer gossip")
	share := flag.Bool("share", true, "advertise this relay via P2P gossip (override with RE_SHARE_RELAY=0)")
	allowWS := flag.Bool("allow-ws", false, "accept ws:// peers in gossip (dev only)")
	flag.Parse()

	hub := NewHub(*public)

	publicURL := strings.TrimSpace(*public)
	if publicURL == "" {
		publicURL = derivePublicRelayURL(*listen)
	}
	hub.public = publicURL

	shareCfg := *share
	sharing := p2p.SharingEnabled(&shareCfg)
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
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWS(hub, w, r)
	})

	if sharing && publicURL != "" && !netutil.IsLoopbackHost(hostOfURL(publicURL)) {
		g := &p2p.Gossiper{
			Store:   store,
			Share:   true,
			Region:  *region,
			Version: "0.1.0",
			LoadFunc: func() int {
				return hub.connectionLoad()
			},
		}
		g.Start()
		log.Printf("p2p share enabled; self=%s", publicURL)
	} else {
		log.Printf("p2p share disabled (or no public URL); peer exchange still serves known addrs")
	}

	log.Printf("RunEverything relay listening on %s (ws path /ws) public=%s", *listen, publicURL)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
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
	return fmt.Sprintf("%s://%s/ws", scheme, net.JoinHostPort(host, port))
}

func handleWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	ws, err := tunnel.DefaultUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade: %v", err)
		return
	}
	c := tunnel.Wrap(ws)
	defer c.Close()

	role := r.URL.Query().Get("role")
	switch role {
	case protocol.RoleAgent:
		serveAgent(hub, c, r)
	case protocol.RoleClient:
		serveClient(hub, c, r)
	default:
		// Allow role negotiation via first message; default reject.
		writeErr(c, "", "bad_role", "query role=agent|client required")
	}
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
	return scheme + "://" + r.Host + "/ws"
}

func serveAgent(hub *Hub, c *tunnel.Conn, r *http.Request) {
	deviceID := ""
	defer func() {
		if deviceID != "" {
			hub.clearAgent(deviceID, c)
		}
	}()

	for {
		mt, msg, err := c.WS.ReadMessage()
		if err != nil {
			return
		}
		if mt == websocket.BinaryMessage {
			// Agent binary frames are stdout for a session → forward to client.
			_, sid, _, err := protocol.DecodePTYFrame(msg)
			if err != nil {
				continue
			}
			if deviceID == "" {
				continue
			}
			cli := hub.clientForSession(deviceID, sid)
			if cli != nil {
				_ = cli.WriteBinary(msg)
			}
			continue
		}

		env, err := protocol.Decode(msg)
		if err != nil {
			writeErr(c, "", "bad_json", err.Error())
			continue
		}

		switch env.Type {
		case protocol.TypeRegister:
			var data protocol.RegisterData
			if err := protocol.DecodeData(env, &data); err != nil {
				writeErr(c, env.ID, "bad_data", err.Error())
				continue
			}
			if data.DeviceID == "" || data.DeviceSecret == "" {
				writeErr(c, env.ID, "bad_data", "device_id and device_secret required")
				continue
			}
			if err := hub.setAgent(data.DeviceID, data.DeviceSecret, data.Name, c); err != nil {
				writeErr(c, env.ID, "auth_failed", err.Error())
				return
			}
			deviceID = data.DeviceID
			b, _ := protocol.Encode(protocol.TypeRegisterOK, env.ID, protocol.RegisterOKData{DeviceID: deviceID})
			_ = c.WriteText(b)
			log.Printf("agent online: %s (%s)", data.Name, deviceID)

		case protocol.TypePairOffer:
			if deviceID == "" {
				writeErr(c, env.ID, "not_registered", "register first")
				continue
			}
			var data protocol.PairOfferData
			if err := protocol.DecodeData(env, &data); err != nil {
				writeErr(c, env.ID, "bad_data", err.Error())
				continue
			}
			relay := data.Relay
			if relay == "" {
				relay = publicRelay(hub, r)
			}
			hub.auth.PutPairing(deviceID, data.Name, relay, data.PairingToken, time.Until(time.Unix(data.ExpiresAt, 0)))
			// ack with same payload shape
			b, _ := protocol.Encode(protocol.TypePairAck, env.ID, protocol.PairAckData{
				DeviceID: deviceID,
				Name:     data.Name,
				Relay:    relay,
			})
			_ = c.WriteText(b)

		case protocol.TypeSessionReady, protocol.TypeSessionClose, protocol.TypeError, protocol.TypePong, protocol.TypePing:
			// Forward control messages about sessions to the bound client.
			if deviceID == "" {
				continue
			}
			var sid string
			switch env.Type {
			case protocol.TypeSessionReady:
				var d protocol.SessionReadyData
				_ = protocol.DecodeData(env, &d)
				sid = d.SessionID
			case protocol.TypeSessionClose:
				var d protocol.SessionCloseData
				_ = protocol.DecodeData(env, &d)
				sid = d.SessionID
			case protocol.TypeResize:
				var d protocol.ResizeData
				_ = protocol.DecodeData(env, &d)
				sid = d.SessionID
			}
			if sid != "" {
				if cli := hub.clientForSession(deviceID, sid); cli != nil {
					_ = cli.WriteText(msg)
				}
				if env.Type == protocol.TypeSessionClose {
					hub.unbindSession(deviceID, sid)
				}
			} else if env.Type == protocol.TypePing {
				b, _ := protocol.Encode(protocol.TypePong, env.ID, nil)
				_ = c.WriteText(b)
			}

		default:
			// Forward unknown control to all clients? Prefer session-targeted only.
			log.Printf("agent msg type=%s device=%s", env.Type, deviceID)
		}
	}
}

func serveClient(hub *Hub, c *tunnel.Conn, r *http.Request) {
	deviceID := ""
	defer func() {
		if deviceID != "" {
			hub.removeClient(deviceID, c)
		}
	}()

	for {
		mt, msg, err := c.WS.ReadMessage()
		if err != nil {
			return
		}
		if mt == websocket.BinaryMessage {
			if deviceID == "" {
				continue
			}
			agent := hub.agentOf(deviceID)
			if agent != nil {
				_ = agent.WriteBinary(msg)
			}
			continue
		}

		env, err := protocol.Decode(msg)
		if err != nil {
			writeErr(c, "", "bad_json", err.Error())
			continue
		}

		switch env.Type {
		case protocol.TypePairRedeem:
			var data protocol.PairRedeemData
			if err := protocol.DecodeData(env, &data); err != nil {
				writeErr(c, env.ID, "bad_data", err.Error())
				continue
			}
			ack, ok := hub.auth.RedeemPairing(data.DeviceID, data.PairingToken)
			if !ok {
				writeErr(c, env.ID, "pair_failed", "invalid or expired pairing token")
				continue
			}
			b, _ := protocol.Encode(protocol.TypePairAck, env.ID, protocol.PairAckData{
				DeviceID:     ack.DeviceID,
				SessionToken: ack.SessionToken,
				Name:         ack.Name,
				Relay:        ack.Relay,
			})
			_ = c.WriteText(b)

		case protocol.TypeClientHello:
			var data protocol.ClientHelloData
			if err := protocol.DecodeData(env, &data); err != nil {
				writeErr(c, env.ID, "bad_data", err.Error())
				continue
			}
			if !hub.auth.ValidateSession(data.DeviceID, data.SessionToken) {
				writeErr(c, env.ID, "auth_failed", "invalid session token")
				return
			}
			deviceID = data.DeviceID
			hub.addClient(deviceID, c)
			_, online := hub.deviceOnline(deviceID)
			b, _ := protocol.Encode(protocol.TypeClientOK, env.ID, protocol.ClientOKData{
				DeviceID: deviceID,
				Online:   online,
			})
			_ = c.WriteText(b)

		case protocol.TypeSessionOpen:
			if deviceID == "" {
				writeErr(c, env.ID, "not_authed", "client_hello required")
				continue
			}
			var data protocol.SessionOpenData
			if err := protocol.DecodeData(env, &data); err != nil {
				writeErr(c, env.ID, "bad_data", err.Error())
				continue
			}
			if data.SessionID == "" {
				writeErr(c, env.ID, "bad_data", "session_id required")
				continue
			}
			agent := hub.agentOf(deviceID)
			if agent == nil {
				writeErr(c, env.ID, "offline", "device agent offline")
				continue
			}
			hub.bindSession(deviceID, data.SessionID, c)
			_ = agent.WriteText(msg)

		case protocol.TypeSessionClose, protocol.TypeResize:
			if deviceID == "" {
				continue
			}
			agent := hub.agentOf(deviceID)
			if agent == nil {
				continue
			}
			if env.Type == protocol.TypeSessionClose {
				var d protocol.SessionCloseData
				_ = protocol.DecodeData(env, &d)
				hub.unbindSession(deviceID, d.SessionID)
			}
			_ = agent.WriteText(msg)

		case protocol.TypePing:
			b, _ := protocol.Encode(protocol.TypePong, env.ID, nil)
			_ = c.WriteText(b)

		default:
			raw, _ := json.Marshal(env)
			log.Printf("client msg: %s", raw)
		}
	}
}
