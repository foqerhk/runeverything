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

	if err := a.connectOnceRE2(); err != nil {
		return err
	}
	defer a.re2Conn.Close()

	if err := a.registerRE2(); err != nil {
		return err
	}
	i18n.Log("log.re2_registered", a.id.Name, a.id.DeviceID, a.cfg.RelayURL)

	if err := a.offerPairRE2(a.printQR); err != nil {
		i18n.Log("log.re2_pair_offer", err)
	}
	a.printQR = false

	done := make(chan struct{})
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

	for {
		f, err := a.re2Conn.ReadFrame()
		if err != nil {
			return err
		}
		switch f.Type {
		case re2.TypeNoise:
			if a.re2Sess != nil {
				log.Printf("re2 unexpected NOISE after session device=%s len=%d", a.id.DeviceID, len(f.Payload))
				continue
			}
			if a.pairingToken == "" {
				log.Printf("re2 NOISE before pair offer; ignoring")
				continue
			}
			psk := re2.DerivePSK(a.pairingToken)
			hs, err := re2.NewAgentHandshake(psk, a.noiseKP)
			if err != nil {
				i18n.Log("log.re2_handshake_init", err)
				continue
			}
			tr := &pendingNoiseTransport{first: f.Payload, conn: a.re2Conn, routeID: a.id.DeviceID}
			sess, _, err := hs.RunAgent(tr)
			if err != nil {
				i18n.Log("log.re2_handshake_fail", err)
				continue
			}
			a.re2Sess = sess
			i18n.Log("log.re2_noise_ok", a.id.DeviceID)

		case re2.TypeTunnel:
			if a.re2Sess == nil {
				log.Printf("re2 TUNNEL before handshake device=%s len=%d", a.id.DeviceID, len(f.Payload))
				continue
			}
			plain, err := a.re2Sess.Decrypt(f.Payload)
			if err != nil {
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
				a.re2Sess = nil
				a.closeAllSessionsOnly()
			}

		default:
			i18n.Log("log.re2_unknown_frame", re2.FrameTypeName(f.Type))
		}
	}
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
		}
	}
	return nil
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

func (a *Agent) offerPairRE2(print bool) error {
	p, token, err := pairing.NewPayloadOpts(
		a.cfg.PublicRelay,
		a.id.DeviceID,
		a.id.Name,
		pairing.DefaultTTL,
		pairing.Options{
			NoisePub: identity.NoisePublicB64URL(a.noiseKP),
			Version:  protocol.Version,
			UDP:      a.udpHostPort,
		},
	)
	if err != nil {
		return err
	}
	a.pairingToken = token
	a.re2Sess = nil // new offer invalidates prior E2E (new PSK)

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
		return a.sendRE2Inner(re2.MsgPong, body)

	case re2.MsgPong:
		// ignore

	case re2.MsgOpenDesktop:
		var data re2.OpenDesktopPayload
		if err := json.Unmarshal(body, &data); err != nil {
			return a.sendRE2AppErr("bad_data", err.Error())
		}
		if err := a.openDesktop(data); err != nil {
			return a.sendRE2AppErr("desktop_open_failed", err.Error())
		}

	case re2.MsgDesktopClose:
		var data re2.DesktopClosePayload
		_ = json.Unmarshal(body, &data)
		a.closeDesktop()

	case re2.MsgInputMouse, re2.MsgInputKey, re2.MsgInputTouch:
		a.touchActivity()
		return a.handleDesktopInput(mt, body)

	case re2.MsgStats:
		var st re2.StatsPayload
		if err := json.Unmarshal(body, &st); err != nil {
			return err
		}
		a.mu.Lock()
		abr := a.deskABR
		a.mu.Unlock()
		if abr != nil {
			abr.OnStats(st.RTTMs, st.LossPct, st.WantKeyframe)
		}

	case re2.MsgKeyframeReq:
		a.mu.Lock()
		if a.deskABR != nil {
			a.deskABR.RequestKeyframe()
		}
		a.mu.Unlock()

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
			out = append(out, re2.DisplayInfo{ID: m.ID, Name: m.Name, Width: m.Width, Height: m.Height, X: m.X, Y: m.Y, Primary: m.Primary})
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
		re2.MsgUSBList, re2.MsgUSBAttach, re2.MsgUSBDetach,
		re2.MsgPrinterList, re2.MsgPrinterJob,
		re2.MsgFileList, re2.MsgInputMode:
		return a.handlePeripheral(mt, body)

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
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, s := range a.sessions {
		_ = s.Close()
		delete(a.sessions, id)
	}
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

