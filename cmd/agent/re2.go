package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/netutil"
	"github.com/foqerhk/runeverything/internal/pairing"
	"github.com/foqerhk/runeverything/internal/protocol"
	ptyx "github.com/foqerhk/runeverything/internal/pty"
	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/registrar"
	"github.com/foqerhk/runeverything/internal/tunnel"
)

// pendingNoiseTransport feeds an already-read Noise payload as the first Recv.
type pendingNoiseTransport struct {
	first   []byte
	conn    *re2.Conn
	routeID string
}

func (t *pendingNoiseTransport) Send(msg []byte) error {
	return t.conn.WriteFrame(re2.Frame{Type: re2.TypeNoise, RouteID: t.routeID, Payload: msg})
}

func (t *pendingNoiseTransport) Recv() ([]byte, error) {
	if t.first != nil {
		m := t.first
		t.first = nil
		return m, nil
	}
	tr := &re2.FrameNoiseTransport{Conn: t.conn, RouteID: t.routeID}
	return tr.Recv()
}

func (a *Agent) runLoopRE2() error {
	noiseKP, err := identity.LoadOrCreateNoiseStatic()
	if err != nil {
		return err
	}
	a.noiseKP = noiseKP
	applyRE2Paths(a.cfg)

	// Fresh signaling/UDP generation — drop any prior Noise session so a new
	// client handshake is not mis-decrypted as tunnel ciphertext.
	a.clearCryptoSession()

	if err := a.connectOnceRE2(); err != nil {
		return err
	}
	defer a.re2Conn.Close()

	if err := a.registerRE2(); err != nil {
		return err
	}
	i18n.Log("log.re2_registered", a.id.Name, a.id.DeviceID, a.cfg.RelayURL)

	// Prefer the last offered token (disk) so App reconnect PSK still matches after Agent restart.
	a.restorePairingTokenIfNeeded()

	// Reuse an unexpired pairing token across reconnects. Rotating here would
	// invalidate any QR already printed to the terminal while still "within expiry".
	rotate := a.pairingToken == "" || a.pairingExpiresAt <= time.Now().Unix()
	if err := a.offerPairRE2(a.printQR, rotate); err != nil {
		i18n.Log("log.re2_pair_offer", err)
	}
	a.printQR = false

	done := make(chan struct{})
	a.startSecondaryRelays(done)
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				_ = a.re2Conn.WriteFrame(re2.Frame{Type: re2.TypePing, RouteID: a.id.DeviceID})
			}
		}
	}()
	defer close(done)
	// If the relay socket drops, always stop capture/control so tray + OS
	// screen-sharing state cannot stick after the phone is already gone.
	defer a.endRemoteControl()

	for {
		f, err := a.re2Conn.ReadFrame()
		if err != nil {
			return err
		}
		switch f.Type {
		case re2.TypeNoise:
			if a.pairingToken == "" {
				log.Printf("re2 NOISE before pair offer; ignoring")
				continue
			}
			// Never let a WSS peer steal an active UDP media session. For an
			// existing WSS session, however, a fresh authenticated Noise handshake
			// is the only recovery from an outbound nonce burn: refusing it leaves
			// the phone permanently stuck in handshaking.
			a.mu.Lock()
			deskLive := a.deskSID != ""
			hasSess := a.re2Sess != nil
			useUDP := a.useUDP
			a.mu.Unlock()
			if hasSess || deskLive {
				if useUDP {
					log.Printf("re2 WSS Noise ignored — UDP session already live device=%s desk=%v sess=%v",
						a.id.DeviceID, deskLive, hasSess)
					continue
				}
				log.Printf("re2 WSS rekey requested — replacing stale session device=%s desk=%v", a.id.DeviceID, deskLive)
				a.closeDesktop()
				a.mu.Lock()
				a.re2Sess = nil
				a.videoMedia = nil
				a.mu.Unlock()
			}
			tr := &pendingNoiseTransport{first: f.Payload, conn: a.re2Conn, routeID: a.id.DeviceID}
			sess, _, _, err := a.runAgentNoise(tr)
			if err != nil {
				i18n.Log("log.re2_handshake_fail", err)
				continue
			}
			a.re2Sess = sess
			i18n.Log("log.re2_noise_ok", a.id.DeviceID)
			log.Printf("re2 media plane=WSS·Relay device=%s", a.id.DeviceID)
			trayCloseQRWindow()

		case re2.TypeTunnel:
			if a.re2Sess == nil {
				log.Printf("re2 TUNNEL before handshake device=%s len=%d", a.id.DeviceID, len(f.Payload))
				continue
			}
			plain, err := a.re2Sess.Decrypt(f.Payload)
			if err != nil {
				// One corrupt/out-of-order client packet (e.g. raced mouse encrypt) must NOT
				// tear down the Noise session — that freezes outbound desktop video until
				// the client re-handshakes. Drop the bad frame and keep streaming.
				i18n.Log("log.re2_decrypt_fail", err)
				continue
			}
			if err := a.handleRE2Inner(plain); err != nil {
				i18n.Log("log.re2_inner", err)
			}

		case re2.TypePing:
			_ = a.re2Conn.WriteFrame(re2.Frame{Type: re2.TypePong, RouteID: f.RouteID, Payload: f.Payload})

		case re2.TypePong:
			// ignore

		case re2.TypeError:
			var ed re2.ErrorPayload
			_ = json.Unmarshal(f.Payload, &ed)
			i18n.Log("log.re2_relay_error", ed.Code, ed.Message)
			if ed.Code == "peer_gone" {
				// BIND and the UDP media plane have independent lifetimes. On cellular,
				// a delayed close from the pair/redeem socket can arrive just after the
				// new BIND + UDP·Relay handshake. Killing the live UDP session here made
				// 5G freeze after its first frame. Preserve any active UDP path (direct
				// or relay) while authenticated traffic is recent; stale sessions still
				// get the normal delayed cleanup.
				if a.useUDP && a.udpEP != nil {
					a.mu.Lock()
					ago := time.Duration(0)
					recent := false
					if !a.lastActivity.IsZero() {
						ago = time.Since(a.lastActivity)
						recent = ago < 8*time.Second
					}
					a.mu.Unlock()
					if recent {
						log.Printf("re2 peer_gone — keep active UDP (direct=%v activity %v ago) device=%s",
							a.udpEP.UsingDirect(),
							ago.Round(time.Millisecond), a.id.DeviceID)
						continue
					}
					log.Printf("re2 peer_gone — arm UDP stale clear direct=%v device=%s",
						a.udpEP.UsingDirect(), a.id.DeviceID)
					a.armUDPSessionStale(3 * time.Second)
					continue
				}
				a.re2Sess = nil
				a.videoMedia = nil
				a.useUDP = false
				// Client left: stop desktop capture (macOS screen-sharing indicator)
				// and PTY sessions; do not tear down the agent↔relay socket.
				a.endRemoteControl()
				// Re-publish the same QR token so the next scan/pairRedeem works
				// without forcing the user to mint a new code.
				_ = a.offerPairRE2(false, false)
			}

		default:
			i18n.Log("log.re2_unknown_frame", re2.FrameTypeName(f.Type))
		}
	}
}

func (a *Agent) clearCryptoSession() {
	a.mu.Lock()
	a.re2Sess = nil
	a.videoMedia = nil
	a.useUDP = false
	if a.udpEP != nil {
		_ = a.udpEP.Close()
		a.udpEP = nil
	}
	a.mu.Unlock()
}

func (a *Agent) restorePairingTokenIfNeeded() {
	if a.pairingToken != "" && a.pairingExpiresAt > time.Now().Unix() {
		return
	}
	home, err := identity.HomeDir()
	if err != nil {
		return
	}
	b, err := os.ReadFile(filepath.Join(home, "last_pairing.json"))
	if err != nil {
		return
	}
	var p protocol.PairingPayload
	if json.Unmarshal(b, &p) != nil || p.PairingToken == "" {
		return
	}
	if p.ExpiresAt > 0 && p.ExpiresAt <= time.Now().Unix() {
		return
	}
	a.pairingToken = p.PairingToken
	a.pairingExpiresAt = p.ExpiresAt
}

func (a *Agent) connectOnceRE2() error {
	u, err := url.Parse(a.cfg.RelayURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("role", protocol.RoleAgent)
	u.RawQuery = q.Encode()
	tc, _, err := tunnel.Dial(u.String(), nil)
	if err != nil {
		registrar.ReportBadAsync(a.cfg.RelayURL, "dial_fail", err.Error(), a.id.DeviceID)
		return err
	}
	a.re2Conn = re2.WrapWS(tc.WS)
	return nil
}

func (a *Agent) registerRE2() error {
	osName, arch := identity.PlatformInfo()
	payload := re2.MustJSON(re2.RegisterPayload{
		DeviceID:     a.id.DeviceID,
		DeviceSecret: a.id.DeviceSecret,
		Name:         a.id.Name,
		OS:           osName,
		Arch:         arch,
		NoisePub:     identity.NoisePublicB64URL(a.noiseKP),
	})
	if err := a.re2Conn.WriteFrame(re2.Frame{
		Type:    re2.TypeRegister,
		RouteID: a.id.DeviceID,
		Payload: payload,
	}); err != nil {
		return err
	}
	_ = a.re2Conn.WS.SetReadDeadline(time.Now().Add(15 * time.Second))
	f, err := a.re2Conn.ReadFrame()
	_ = a.re2Conn.WS.SetReadDeadline(time.Time{})
	if err != nil {
		return err
	}
	if f.Type == re2.TypeError {
		var ed re2.ErrorPayload
		_ = json.Unmarshal(f.Payload, &ed)
		return fmt.Errorf("register failed: %s", ed.Message)
	}
	if f.Type != re2.TypeRegisterOK {
		return fmt.Errorf("unexpected register response type=%s", re2.FrameTypeName(f.Type))
	}
	var rok re2.RegisterOKPayload
	_ = json.Unmarshal(f.Payload, &rok)
	udp := rok.UDP
	if udp == "" {
		udp = udpFromRelayURL(a.cfg.RelayURL)
	}
	a.udpHostPort = udp
	if udp != "" {
		if err := a.startUDP(udp); err != nil {
			i18n.Log("log.reudp_assoc_warn", err)
			// Keep retrying in background — client ASSOC needs the agent on UDP.
			go a.retryUDPInBackground(udp)
		}
	}
	return nil
}

func (a *Agent) retryUDPInBackground(udpHostPort string) {
	for attempt := 1; attempt <= 8; attempt++ {
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
		a.mu.Lock()
		already := a.udpEP != nil
		a.mu.Unlock()
		if already {
			return
		}
		if err := a.startUDP(udpHostPort); err != nil {
			i18n.Log("log.reudp_assoc_warn", err)
			continue
		}
		// Refresh QR lan ports now that listen is up (avoid stale / empty lan on App).
		_ = a.offerPairRE2(false, false)
		return
	}
}

func udpFromRelayURL(relay string) string {
	u, err := url.Parse(relay)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "8787"
	}
	if host == "" {
		return ""
	}
	return net.JoinHostPort(host, port)
}

// offerPairRE2 publishes a pairing token to the relay.
// When rotate is false and a non-expired token exists, the same token is re-advertised
// so a previously printed QR keeps working after agent reconnect.
func (a *Agent) offerPairRE2(print, rotate bool) error {
	if a.re2Conn == nil {
		return fmt.Errorf("not connected to relay")
	}
	noisePub := identity.NoisePublicB64URL(a.noiseKP)
	udp := a.udpHostPort
	lan := a.lanPairingCandidates()
	qrRelay := re2.EnsurePath(netutil.PairingAdvertisedRelay(a.cfg.PublicRelay, a.cfg.RelayURL))
	if netutil.RelayURLHostIsUnreliableOnWAN(qrRelay) {
		log.Printf("re2 pair QR relay still LAN-only (%s) — set public_relay in Agent config for cellular", qrRelay)
	}

	var p *protocol.PairingPayload
	var token string
	if !rotate && a.pairingToken != "" && a.pairingExpiresAt > time.Now().Unix() {
		token = a.pairingToken
		p = &protocol.PairingPayload{
			V:            protocol.Version,
			Relay:        qrRelay,
			DeviceID:     a.id.DeviceID,
			PairingToken: token,
			Name:         a.id.Name,
			ExpiresAt:    a.pairingExpiresAt,
			NoisePub:     noisePub,
			UDP:          udp,
			LAN:          lan,
		}
	} else {
		var err error
		p, token, err = pairing.NewPayloadOpts(
			qrRelay,
			a.id.DeviceID,
			a.id.Name,
			pairing.DefaultTTL,
			pairing.Options{
				NoisePub: noisePub,
				Version:  protocol.Version,
				UDP:      udp,
				LAN:      lan,
			},
		)
		if err != nil {
			return err
		}
		// Live sessions stay up: earlier tokens remain valid handshake PSKs.
		noteIssuedToken(token)
	}
	a.pairingToken = token
	a.pairingExpiresAt = p.ExpiresAt
	p.Relays = a.pairingRelayCandidates()
	p.UDP = pickNonEmpty(p.UDP, a.udpHostPort)

	if err := a.re2Conn.WriteFrame(re2.Frame{
		Type:    re2.TypePairOffer,
		RouteID: a.id.DeviceID,
		Payload: re2.MustJSON(re2.PairOfferPayload{
			DeviceID:     a.id.DeviceID,
			PairingToken: token,
			Name:         a.id.Name,
			ExpiresAt:    p.ExpiresAt,
			NoisePub:     p.NoisePub,
		}),
	}); err != nil {
		return err
	}
	a.broadcastPairOffer()
	home, _ := identity.EnsureHome()
	_ = os.WriteFile(filepath.Join(home, "last_pairing.json"), mustJSON(p), 0o600)
	if print || a.printQR {
		return pairing.PrintQR(p)
	}
	return nil
}

func (a *Agent) handleRE2Inner(plain []byte) error {
	mt, body, err := re2.DecodeInner(plain)
	if err != nil {
		return err
	}
	switch mt {
	case re2.MsgOpenSession:
		var data re2.OpenSessionPayload
		if err := json.Unmarshal(body, &data); err != nil {
			return a.sendRE2AppErr("bad_data", err.Error())
		}
		if err := a.openSessionRE2(data); err != nil {
			return a.sendRE2AppErr("session_open_failed", err.Error())
		}
		return a.sendRE2Inner(re2.MsgSessionReady, re2.MustJSON(re2.SessionReadyPayload{SessionID: data.SessionID}))

	case re2.MsgSessionClose:
		var data re2.SessionClosePayload
		_ = json.Unmarshal(body, &data)
		a.closeSession(data.SessionID, data.Reason)

	case re2.MsgResize:
		var data re2.ResizePayload
		_ = json.Unmarshal(body, &data)
		a.mu.Lock()
		s := a.sessions[data.SessionID]
		a.mu.Unlock()
		if s != nil {
			_ = s.Resize(data.Cols, data.Rows)
		}

	case re2.MsgPTYData:
		sid, data, err := re2.DecodePTY(body)
		if err != nil {
			return err
		}
		a.mu.Lock()
		s := a.sessions[sid]
		a.mu.Unlock()
		if s != nil {
			_, _ = s.Write(data)
		}

	case re2.MsgPing:
		a.touchActivity()
		return a.sendRE2Inner(re2.MsgPong, body)

	case re2.MsgPong:
		// ignore

	case re2.MsgOpenDesktop:
		var data re2.OpenDesktopPayload
		if err := json.Unmarshal(body, &data); err != nil {
			return a.sendRE2AppErr("bad_data", err.Error())
		}
		// Never block the UDP/WSS reader on capture Start / VT create — a synchronous
		// openDesktop hung PreferDirect ACKs so the App saw quality OPEN then 0 RX
		// (no DESKTOP_READY / no video) while Agent was stuck in openDesktop.
		go func(data re2.OpenDesktopPayload) {
			if err := a.openDesktop(data); err != nil {
				log.Printf("desktop OPEN async failed: %v", err)
				_ = a.sendRE2AppErr("desktop_open_failed", err.Error())
			}
		}(data)

	case re2.MsgDesktopClose:
		var data re2.DesktopClosePayload
		_ = json.Unmarshal(body, &data)
		a.closeDesktop()
		// Explicit client goodbye (background / net drop) — clear Noise too so the
		// next resume is not "WSS Noise ignored — session already live" / stale-clear.
		switch data.Reason {
		case "app_background", "network_change", "user_disconnect":
			a.cancelUDPSessionStale()
			a.mu.Lock()
			a.re2Sess = nil
			a.videoMedia = nil
			a.useUDP = false
			a.mu.Unlock()
			if a.udpEP != nil {
				a.udpEP.ClearDirect()
				a.udpEP.ResetReliableSession()
			}
			log.Printf("re2 client goodbye reason=%s — desktop+Noise cleared device=%s",
				data.Reason, a.id.DeviceID)
		}

	case re2.MsgInputMouse, re2.MsgInputKey, re2.MsgInputTouch:
		a.touchActivity()
		a.enqueueDesktopInput(mt, body)
		return nil

	case re2.MsgStats:
		a.touchActivity()
		var st re2.StatsPayload
		if err := json.Unmarshal(body, &st); err != nil {
			return err
		}
		a.mu.Lock()
		abr := a.deskABR
		a.mu.Unlock()
		if abr != nil {
			abr.OnStatsSample(desktop.StatsSample{
				RTTMs:         st.RTTMs,
				LossPct:       st.LossPct,
				WantKeyframe:  st.WantKeyframe,
				RecvKbps:      st.RecvKbps,
				JitterMs:      st.JitterMs,
				DecodeDelayMs: st.DecodeDelayMs,
				Stall:         st.Stall,
				StreamFPS:     st.StreamFPS,
			})
		}

	case re2.MsgKeyframeReq:
		a.touchActivity()
		a.mu.Lock()
		if a.deskABR != nil {
			a.deskABR.RequestKeyframe()
		}
		a.mu.Unlock()

	case re2.MsgVideoNACK:
		a.touchActivity()
		var req re2.VideoNACKPayload
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		a.resendVideoParts(req)

	case re2.MsgClipboard:
		var cp re2.ClipboardPayload
		if err := json.Unmarshal(body, &cp); err != nil {
			return err
		}
		if a.clip == nil {
			a.clip = desktop.NewClipboardHub()
		}
		if cp.Text != "" {
			a.clip.SetText(cp.Text)
		}
		if cp.DataB64 != "" && (cp.Mime == "image/png" || strings.HasPrefix(cp.Mime, "image/")) {
			if b, err := desktop.DecodePNGB64(cp.DataB64); err == nil {
				a.clip.SetPNG(b)
			}
		}

	case re2.MsgAudio:
		var ap re2.AudioPayload
		if err := json.Unmarshal(body, &ap); err != nil {
			return err
		}
		raw, err := base64.StdEncoding.DecodeString(ap.DataB64)
		if err != nil || len(raw) == 0 {
			return nil
		}
		sr := ap.SampleRate
		if sr <= 0 {
			sr = 48000
		}
		codec := ap.Codec
		if codec == "" {
			codec = "pcm16"
		}
		a.mu.Lock()
		player := a.audioPlayer
		if player == nil {
			p, err := desktop.StartAudioPlayer(context.Background(), sr, codec)
			if err == nil && p != nil {
				a.audioPlayer = p
				player = p
			}
		}
		a.mu.Unlock()
		if player != nil {
			_ = player.Write(raw)
		} else {
			_ = desktop.PlayPCM16(sr, raw)
		}

	case re2.MsgDisplays:
		var dp re2.DisplaysPayload
		if err := json.Unmarshal(body, &dp); err != nil {
			return err
		}
		if dp.Action == "select" {
			desktop.SetSelectedMonitor(dp.DisplayID)
		}
		mons, _ := desktop.ListMonitors()
		out := make([]re2.DisplayInfo, 0, len(mons))
		for _, m := range mons {
			di := re2.DisplayInfo{ID: m.ID, Name: m.Name, Width: m.Width, Height: m.Height, X: m.X, Y: m.Y, Primary: m.Primary}
			if desktop.IsVirtualDisplay(m.ID) {
				di.Virtual = true
				if fw, fh, ok := desktop.VirtualFramebuffer(m.ID); ok {
					di.FBWidth, di.FBHeight = fw, fh
				}
			}
			out = append(out, di)
		}
		return a.sendTunnel(re2.MsgDisplays, re2.MustJSON(re2.DisplaysPayload{Action: "list", Displays: out}), true)

	case re2.MsgHolePunch:
		var hp re2.HolePunchPayload
		if err := json.Unmarshal(body, &hp); err != nil {
			return err
		}
		return a.handleHolePunch(hp)

	case re2.MsgFileOffer, re2.MsgFileChunk, re2.MsgFilePull:
		return a.handleFileMsg(mt, body)

	case re2.MsgWakeOnLAN, re2.MsgCameraList, re2.MsgCameraOpen, re2.MsgCameraClose,
		re2.MsgPhoneCamOpen, re2.MsgPhoneCamClose, re2.MsgPhoneCamFrame,
		re2.MsgUSBList, re2.MsgUSBAttach, re2.MsgUSBDetach,
		re2.MsgPrinterList, re2.MsgPrinterJob,
		re2.MsgFileList, re2.MsgInputMode:
		return a.handlePeripheral(mt, body)

	case re2.MsgAgentChatList, re2.MsgAgentChatDetail:
		// Data-only AI session inventory — does not open desktop / capture / inject.
		// Run off the RE2 read loop: agentchat.List can stall the filesystem and
		// would otherwise delay mouse decrypt / enqueue under load.
		bodyCopy := append([]byte(nil), body...)
		go func() {
			if err := a.handleAgentChat(mt, bodyCopy); err != nil {
				log.Printf("agent chat: %v", err)
			}
		}()
		return nil

	default:
		i18n.Log("log.re2_unknown_inner", mt)
	}
	return nil
}

func (a *Agent) openSessionRE2(data re2.OpenSessionPayload) error {
	return a.openSession(data)
}

func (a *Agent) sendRE2Inner(msgType byte, body []byte) error {
	return a.sendTunnel(msgType, body, true)
}

func (a *Agent) sendRE2AppErr(code, msg string) error {
	return a.sendRE2Inner(re2.MsgAppError, re2.MustJSON(re2.ErrorPayload{Code: code, Message: msg}))
}

func (a *Agent) closeAllSessionsOnly() {
	a.endRemoteControl()
}

// endRemoteControl stops desktop capture/inject and all PTY sessions, and clears
// "being controlled" UI state. Safe to call repeatedly.
func (a *Agent) endRemoteControl() {
	a.closeDesktop()
	a.mu.Lock()
	for id, s := range a.sessions {
		_ = s.Close()
		delete(a.sessions, id)
	}
	a.controlPeer = ""
	a.controlSince = time.Time{}
	a.controlDeskOn = false
	// Drop LAN PreferDirect — the client's UDP port dies with the app process.
	// Stale directAddr makes the next Noise msg2 vanish (handshake never completes).
	if a.udpEP != nil {
		a.udpEP.ClearDirect()
	}
	a.mu.Unlock()
}

// pumpStdoutRE2 is used when RE2 session is active (override binary pump).
func (a *Agent) pumpStdoutRE2(s *ptyx.Session) {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			if a.re2Sess == nil || a.re2Conn == nil {
				break
			}
			body := re2.EncodePTY(s.ID, buf[:n])
			if werr := a.sendRE2Inner(re2.MsgPTYData, body); werr != nil {
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				i18n.Log("log.session_read", s.ID, err)
			}
			a.closeSession(s.ID, "pty_exit")
			_ = a.sendRE2Inner(re2.MsgSessionClose, re2.MustJSON(re2.SessionClosePayload{
				SessionID: s.ID,
				Reason:    "pty_exit",
			}))
			return
		}
	}
}
