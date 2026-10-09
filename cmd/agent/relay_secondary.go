package main

import (
	"encoding/json"
	"log"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/protocol"
	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/tunnel"
)

func udpHintFromRelayURL(relay string) string {
	u, err := url.Parse(strings.TrimSpace(relay))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		port = "8787"
	}
	return net.JoinHostPort(u.Hostname(), port)
}

func (a *Agent) pairingRelayCandidates() []protocol.RelayCandidate {
	primary := strings.TrimSpace(a.cfg.PublicRelay)
	if primary == "" {
		primary = strings.TrimSpace(a.cfg.RelayURL)
	}
	out := []protocol.RelayCandidate{{
		Relay: primary,
		UDP:   pickNonEmpty(a.udpHostPort, udpHintFromRelayURL(primary)),
	}}
	seen := map[string]bool{primary: true}
	for _, raw := range a.altRelays {
		r := re2.EnsurePath(strings.TrimSpace(raw))
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, protocol.RelayCandidate{
			Relay: r,
			UDP:   udpHintFromRelayURL(r),
		})
	}
	if len(out) <= 1 {
		return nil
	}
	return out
}

func pickNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// startSecondaryRelays keeps REGISTER+PAIR_OFFER on alternate relays and can
// accept a client Noise/session if KoKo failovers via QR relays[].
func (a *Agent) startSecondaryRelays(stop <-chan struct{}) {
	for _, raw := range a.altRelays {
		url := re2.EnsurePath(strings.TrimSpace(raw))
		if url == "" || url == a.cfg.RelayURL || url == a.cfg.PublicRelay {
			continue
		}
		u := url
		go a.secondaryRelayLoop(u, stop)
	}
}

func (a *Agent) secondaryRelayLoop(relayURL string, stop <-chan struct{}) {
	backoff := time.Second
	for {
		select {
		case <-stop:
			return
		default:
		}
		if err := a.secondaryRelayOnce(relayURL, stop); err != nil {
			log.Printf("secondary relay %s: %v; retry in %s", relayURL, err, backoff)
		}
		select {
		case <-stop:
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (a *Agent) secondaryRelayOnce(relayURL string, stop <-chan struct{}) error {
	u, err := url.Parse(relayURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("role", protocol.RoleAgent)
	u.RawQuery = q.Encode()
	tc, _, err := tunnel.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	conn := re2.WrapWS(tc.WS)
	defer conn.Close()

	osName, arch := identity.PlatformInfo()
	if err := conn.WriteFrame(re2.Frame{
		Type:    re2.TypeRegister,
		RouteID: a.id.DeviceID,
		Payload: re2.MustJSON(re2.RegisterPayload{
			DeviceID:     a.id.DeviceID,
			DeviceSecret: a.id.DeviceSecret,
			Name:         a.id.Name,
			OS:           osName,
			Arch:         arch,
			NoisePub:     identity.NoisePublicB64URL(a.noiseKP),
		}),
	}); err != nil {
		return err
	}
	_ = conn.WS.SetReadDeadline(time.Now().Add(15 * time.Second))
	ack, err := conn.ReadFrame()
	if err != nil {
		return err
	}
	if ack.Type != re2.TypeRegisterOK {
		return errString("secondary register: unexpected frame")
	}
	i18n.Log("log.re2_registered", a.id.Name+"@alt", a.id.DeviceID, relayURL)

	a.secondaryMu.Lock()
	a.secondaryConns = append(a.secondaryConns, conn)
	a.secondaryMu.Unlock()
	defer func() {
		a.secondaryMu.Lock()
		out := a.secondaryConns[:0]
		for _, c := range a.secondaryConns {
			if c != conn {
				out = append(out, c)
			}
		}
		a.secondaryConns = out
		a.secondaryMu.Unlock()
	}()

	_ = a.offerPairOnConn(conn)
	offerTick := time.NewTicker(45 * time.Second)
	defer offerTick.Stop()

	for {
		select {
		case <-stop:
			return nil
		case <-offerTick.C:
			_ = a.offerPairOnConn(conn)
		default:
		}
		_ = conn.WS.SetReadDeadline(time.Now().Add(70 * time.Second))
		f, err := conn.ReadFrame()
		if err != nil {
			return err
		}
		switch f.Type {
		case re2.TypePing:
			_ = conn.WriteFrame(re2.Frame{Type: re2.TypePong, RouteID: f.RouteID, Payload: f.Payload})
		case re2.TypeNoise:
			if err := a.acceptNoiseOnConn(conn, relayURL, f.Payload); err != nil {
				i18n.Log("log.re2_handshake_fail", err)
				continue
			}
			// Own the session on this alternate until disconnect.
			return a.serveSecondarySession(conn, relayURL, stop)
		case re2.TypeTunnel:
			// Stale tunnel before Noise on alt — ignore.
		case re2.TypeError:
			// ignore
		}
	}
}

func (a *Agent) acceptNoiseOnConn(conn *re2.Conn, relayURL string, first []byte) error {
	if a.pairingToken == "" {
		return nil
	}
	tr := &pendingNoiseTransport{first: first, conn: conn, routeID: a.id.DeviceID}
	sess, _, _, err := a.runAgentNoise(tr)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.re2Conn = conn
	a.re2Sess = sess
	a.cfg.RelayURL = relayURL
	a.cfg.PublicRelay = relayURL
	if hp := udpHintFromRelayURL(relayURL); hp != "" {
		a.udpHostPort = hp
	}
	a.useUDP = false
	a.lastActivity = time.Now()
	a.mu.Unlock()
	i18n.Log("log.re2_noise_ok", a.id.DeviceID+"@alt")
	_ = a.startUDP(a.udpHostPort)
	return nil
}

func (a *Agent) serveSecondarySession(conn *re2.Conn, relayURL string, stop <-chan struct{}) error {
	log.Printf("secondary relay %s: serving client session", relayURL)
	for {
		select {
		case <-stop:
			return nil
		default:
		}
		_ = conn.WS.SetReadDeadline(time.Now().Add(120 * time.Second))
		f, err := conn.ReadFrame()
		if err != nil {
			a.endRemoteControl()
			return err
		}
		switch f.Type {
		case re2.TypePing:
			_ = conn.WriteFrame(re2.Frame{Type: re2.TypePong, RouteID: f.RouteID, Payload: f.Payload})
		case re2.TypeNoise:
			a.re2Sess = nil
			if err := a.acceptNoiseOnConn(conn, relayURL, f.Payload); err != nil {
				i18n.Log("log.re2_handshake_fail", err)
			}
		case re2.TypeTunnel:
			if a.re2Sess == nil {
				continue
			}
			plain, err := a.re2Sess.Decrypt(f.Payload)
			if err != nil {
				continue
			}
			_ = a.handleRE2Inner(plain)
		case re2.TypeError:
			var ed re2.ErrorPayload
			_ = json.Unmarshal(f.Payload, &ed)
			if ed.Code == "peer_gone" {
				a.re2Sess = nil
				a.endRemoteControl()
			}
		}
	}
}

func (a *Agent) offerPairOnConn(conn *re2.Conn) error {
	if conn == nil || a.pairingToken == "" || a.pairingExpiresAt <= time.Now().Unix() {
		return nil
	}
	return conn.WriteFrame(re2.Frame{
		Type:    re2.TypePairOffer,
		RouteID: a.id.DeviceID,
		Payload: re2.MustJSON(re2.PairOfferPayload{
			DeviceID:     a.id.DeviceID,
			PairingToken: a.pairingToken,
			Name:         a.id.Name,
			ExpiresAt:    a.pairingExpiresAt,
			NoisePub:     identity.NoisePublicB64URL(a.noiseKP),
		}),
	})
}

func (a *Agent) broadcastPairOffer() {
	a.secondaryMu.Lock()
	conns := append([]*re2.Conn(nil), a.secondaryConns...)
	a.secondaryMu.Unlock()
	for _, c := range conns {
		_ = a.offerPairOnConn(c)
	}
}
