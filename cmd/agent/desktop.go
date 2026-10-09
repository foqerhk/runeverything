package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/audit"
	"github.com/foqerhk/runeverything/internal/deskbridge"
	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/holepunch"
	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/identity"
	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/reudp"
)

func (a *Agent) touchActivity() {
	a.mu.Lock()
	a.lastActivity = time.Now()
	a.mu.Unlock()
}

// inlineHEVCEncoder is a no-op Encoder for SCK→VT inline capture (Annex-B already on Frame).
type inlineHEVCEncoder struct{ w, h int }

func (e *inlineHEVCEncoder) Encode(f desktop.Frame, keyframe bool) ([]byte, error) {
	if len(f.AnnexB) > 0 {
		return f.AnnexB, nil
	}
	return nil, errString("inline HEVC: empty annex-B")
}
func (e *inlineHEVCEncoder) Close() error      { return nil }
func (e *inlineHEVCEncoder) CodecName() string { return desktop.CodecH265 }

type desktopInputEvent struct {
	mt   byte
	body []byte
}

type desktopKeyframeCache struct {
	parts [][]byte
	at    time.Time
}

// enqueueDesktopInput keeps pure mouse moves latest-wins under pressure. Discrete
// clicks/keys get a longer bounded wait and are never displaced by move floods.
func (a *Agent) enqueueDesktopInput(mt byte, body []byte) {
	id := inputEventID(mt, body)
	if id > 0 {
		ev := desktopInputEvent{mt: mt, body: body}
		a.mu.Lock()
		if a.inputNextID == 0 {
			a.inputNextID = id
		}
		if id < a.inputNextID {
			a.mu.Unlock()
			return // redundant input-plane copy already executed
		}
		if id > a.inputNextID {
			if a.inputPending == nil {
				a.inputPending = make(map[uint64]desktopInputEvent)
			}
			a.inputPending[id] = ev
			a.mu.Unlock()
			return
		}
		ready := []desktopInputEvent{ev}
		a.inputNextID++
		for {
			next, ok := a.inputPending[a.inputNextID]
			if !ok {
				break
			}
			delete(a.inputPending, a.inputNextID)
			ready = append(ready, next)
			a.inputNextID++
		}
		a.mu.Unlock()
		for _, item := range ready {
			a.queueDesktopInput(item)
		}
		return
	}
	a.queueDesktopInput(desktopInputEvent{mt: mt, body: body})
}

func inputEventID(mt byte, body []byte) uint64 {
	switch mt {
	case re2.MsgInputMouse:
		var p re2.InputMousePayload
		if json.Unmarshal(body, &p) == nil {
			return p.EventID
		}
	case re2.MsgInputKey:
		var p re2.InputKeyPayload
		if json.Unmarshal(body, &p) == nil {
			return p.EventID
		}
	case re2.MsgInputTouch:
		var p re2.InputTouchPayload
		if json.Unmarshal(body, &p) == nil {
			return p.EventID
		}
	}
	return 0
}

func (a *Agent) queueDesktopInput(ev desktopInputEvent) {
	a.mu.Lock()
	ch := a.inputCh
	a.mu.Unlock()
	if ch == nil {
		return
	}
	if ev.mt == re2.MsgInputMouse && mouseMoveOnly(ev.body) {
		select {
		case ch <- ev:
		default:
			// Drop one queued event then try again with the latest move.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
		return
	}
	select {
	case ch <- ev:
	case <-time.After(500 * time.Millisecond):
		log.Printf("input discrete event timed out mt=0x%02x", ev.mt)
	}
}

func mouseMoveOnly(body []byte) bool {
	var p re2.InputMousePayload
	if json.Unmarshal(body, &p) != nil {
		return false
	}
	// Space / gesture packets look like "moves" (no down/up/wheel) but must never
	// be coalesced or latest-wins-dropped — second three-finger swipe would vanish
	// under WSS HOL and leave Dock mid-transition.
	if p.Gesture != "" || p.SpaceDelta != 0 {
		return false
	}
	return !p.Down && !p.Up && p.Wheel == 0 && p.WheelH == 0
}

func coalesceMouseBodies(a, b []byte) []byte {
	var pa, pb re2.InputMousePayload
	if json.Unmarshal(a, &pa) != nil || json.Unmarshal(b, &pb) != nil {
		return b
	}
	if pa.Relative || pb.Relative {
		pb.Relative = true
		pb.DX = pa.DX + pb.DX
		pb.DY = pa.DY + pb.DY
	}
	out, err := json.Marshal(pb)
	if err != nil {
		return b
	}
	return out
}

func (a *Agent) desktopInputPump(ctx context.Context, ch <-chan desktopInputEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			// Coalesce back-to-back pure moves so inject keeps up with video.
			for ev.mt == re2.MsgInputMouse && mouseMoveOnly(ev.body) {
				select {
				case more := <-ch:
					if more.mt == re2.MsgInputMouse && mouseMoveOnly(more.body) {
						ev.body = coalesceMouseBodies(ev.body, more.body)
						continue
					}
					_ = a.handleDesktopInput(ev.mt, ev.body)
					ev = more
				default:
					goto handle
				}
			}
		handle:
			_ = a.handleDesktopInput(ev.mt, ev.body)
		}
	}
}

func (a *Agent) startUDP(udpHostPort string) error {
	if udpHostPort == "" {
		return nil
	}
	// Sticky LAN port: first start picks ephemeral and saves it; later starts reuse
	// so App QR lan:port stays valid without rescanning after Agent restart.
	prefer := identity.PreferredUDPListenPort()
	ep, err := reudp.DialListen(udpHostPort, prefer)
	if err != nil {
		return err
	}
	if p := ep.ListenPort(); p > 0 {
		if err := identity.SaveUDPListenPort(p); err != nil {
			log.Printf("reudp: save sticky listen port %d: %v", p, err)
		} else if prefer == 0 || prefer != p {
			log.Printf("reudp: sticky listen port → %d (QR lan will use this)", p)
		}
	}
	// Retransmit ASSOC — volunteer-relay UDP is often lossy; a single shot times out.
	var ok *reudp.AssocOKPayload
	deadline := time.Now().Add(12 * time.Second)
	for {
		if err := ep.AssocAgent(a.id.DeviceID, a.id.DeviceSecret); err != nil {
			_ = ep.Close()
			return err
		}
		remain := time.Until(deadline)
		if remain <= 0 {
			_ = ep.Close()
			return fmt.Errorf("reudp: assoc timeout")
		}
		wait := remain
		if wait > time.Second {
			wait = time.Second
		}
		ok, err = ep.WaitAssocOK(wait)
		if err == nil {
			break
		}
		if ne, okT := err.(interface{ Timeout() bool }); okT && ne.Timeout() {
			continue
		}
		_ = ep.Close()
		return err
	}
	i18n.Log("log.reudp_associated", ok.DeviceID, ok.UDPHint)
	a.mu.Lock()
	if a.udpEP != nil {
		_ = a.udpEP.Close()
	}
	a.udpEP = ep
	// New UDP association ⇒ peer will Noise again; drop leftover session + desktop
	// so stale WSS video cannot keep encrypting under old keys.
	a.re2Sess = nil
	a.videoMedia = nil
	a.useUDP = false
	a.mu.Unlock()
	a.closeDesktop()
	go a.udpReadLoop()
	return nil
}

// deskLivenessTimeout ends a remote desktop whose phone stopped checking in; the App
// pings every second while viewing, so this only trips once it is really gone.
const deskLivenessTimeout = 15 * time.Second

// livenessLoop stops screen capture when the viewing phone goes silent and releases the
// Noise peer after sessionIdle, so a phone that dropped without saying goodbye neither
// leaves the desktop shared nor blocks the next handshake. PTY sessions keep running
// for the phone to reattach.
// releasePeer drops the current controller's Noise session and desktop so the
// next handshake (from any phone, UDP or WSS) starts clean. PTY sessions stay up.
func (a *Agent) releasePeer() {
	a.mu.Lock()
	a.re2Sess = nil
	a.videoMedia = nil
	a.useUDP = false
	a.lastActivity = time.Time{}
	if a.udpEP != nil {
		a.udpEP.ClearDirect()
	}
	a.mu.Unlock()
	a.cancelUDPSessionStale()
	a.closeDesktop()
}

func (a *Agent) livenessLoop() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for range t.C {
		a.mu.Lock()
		last := a.lastActivity
		desk := a.deskSID != ""
		hasPeer := a.re2Sess != nil
		idle := a.sessionIdle
		a.mu.Unlock()
		if last.IsZero() {
			continue
		}
		silent := time.Since(last)
		switch {
		case hasPeer && idle > 0 && silent > idle:
			i18n.Log("log.session_idle", idle)
			audit.Log("session_idle_timeout", a.id.DeviceID)
			// Keeping stale AEAD made the next reconnect's XX finish MAC-fail.
			a.releasePeer()
		case desk && silent > deskLivenessTimeout:
			log.Printf("re2 desktop: no heartbeat from phone for %v — stopping remote desktop device=%s",
				silent.Round(time.Second), a.id.DeviceID)
			audit.Log("desktop_heartbeat_timeout", a.id.DeviceID)
			a.closeDesktop()
		}
	}
}

func (a *Agent) udpReadLoop() {
	for {
		a.mu.Lock()
		ep := a.udpEP
		a.mu.Unlock()
		if ep == nil {
			return
		}
		payload, err := ep.RecvTimeout(2 * time.Second)
		if err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				continue
			}
			i18n.Log("log.reudp_recv", err)
			return
		}
		if err := a.handleUDPPayload(payload); err != nil {
			log.Printf("reudp handle: %v", err)
		}
	}
}

func (a *Agent) handleUDPPayload(payload []byte) error {
	a.cancelUDPSessionStale()
	if a.re2Sess == nil {
		// Only XX msg1 may start a handshake. Stray ciphertext / retransmits of
		// an aborted session must not be fed to ReadMessage as msg1 (MAC fail
		// loop → App cannot reload desktop after background).
		if !looksLikeNoiseMsg1(payload) {
			return nil
		}
		return a.completeUDPNoise(payload)
	}
	a.touchActivity()
	// Best-effort video plane — never feed into ordered Noise decrypt.
	if re2.IsVideoPlane(payload) {
		a.mu.Lock()
		vm := a.videoMedia
		a.mu.Unlock()
		if vm == nil {
			return nil
		}
		plain, err := vm.Open(payload)
		if err != nil {
			return nil // loss/corrupt video part — ignore
		}
		return a.handleRE2Inner(plain)
	}
	plain, err := a.re2Sess.Decrypt(payload)
	if err != nil {
		// Client may restart Noise after disconnect while we still hold a stale
		// session (peer_gone was deferred on PreferDirect). Treat XX msg1 as
		// re-handshake; otherwise keep the decrypt error.
		if looksLikeNoiseMsg1(payload) {
			log.Printf("reudp: stale Noise decrypt → re-handshake msg1 len=%d", len(payload))
			a.mu.Lock()
			a.re2Sess = nil
			a.videoMedia = nil
			a.useUDP = false
			a.mu.Unlock()
			// Do NOT closeDesktop() here — tearing down 8K/16K libx265 can take
			// seconds and stalls msg2/msg3, so the App times out → WSS·Relay.
			// completeUDPNoise closes the desktop after XX succeeds.
			if err2 := a.completeUDPNoise(payload); err2 == nil {
				return nil
			} else {
				return err2
			}
		}
		return err
	}
	return a.handleRE2Inner(plain)
}

func looksLikeNoiseMsg1(payload []byte) bool {
	// Noise XX msg1 is initiator ephemeral (32) + optional empty payload/MAC.
	n := len(payload)
	return n >= 32 && n <= 80
}

func (a *Agent) armUDPSessionStale(d time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.udpStaleTimer != nil {
		a.udpStaleTimer.Stop()
	}
	marked := a.lastActivity
	a.udpStaleTimer = time.AfterFunc(d, func() {
		a.mu.Lock()
		// Fresh UDP activity since peer_gone ⇒ App still on PreferDirect.
		if !a.lastActivity.Equal(marked) && !a.lastActivity.IsZero() && time.Since(a.lastActivity) < d {
			a.udpStaleTimer = nil
			a.mu.Unlock()
			return
		}
		a.re2Sess = nil
		a.videoMedia = nil
		a.useUDP = false
		a.udpStaleTimer = nil
		a.mu.Unlock()
		log.Printf("re2 UDP·LAN stale clear after peer_gone — ready for clean Noise device=%s", a.id.DeviceID)
		a.closeDesktop()
	})
}

func (a *Agent) cancelUDPSessionStale() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.udpStaleTimer != nil {
		a.udpStaleTimer.Stop()
		a.udpStaleTimer = nil
	}
}

func (a *Agent) completeUDPNoise(payload []byte) error {
	a.udpNoiseMu.Lock()
	defer a.udpNoiseMu.Unlock()
	if a.pairingToken == "" {
		return nil
	}
	// Already completed by a racing packet while we waited for the lock.
	if a.re2Sess != nil {
		return nil
	}
	if a.udpEP != nil {
		a.udpEP.ResetReliableSession()
	}
	tr := &udpPendingTransport{first: payload, ep: a.udpEP}
	sess, peerStatic, psk, err := a.runAgentNoise(tr)
	if err != nil {
		return err
	}
	// New Noise ⇒ new AEAD + App seq space. Drop stale desktop (old pump would
	// Encrypt under mixed keys) and restart our outbound seq at 0. The inbound side
	// was reset before the handshake and may already hold the client's OPEN.
	a.closeDesktop()
	if a.udpEP != nil {
		a.udpEP.ResetReliableSend()
	}
	var vm *re2.VideoMedia
	if a2c, c2a, kerr := re2.DeriveVideoMediaKeys(psk, peerStatic, a.noiseKP.Public); kerr == nil {
		vm, _ = re2.NewVideoMediaAgent(a2c, c2a)
	}
	a.mu.Lock()
	a.re2Sess = sess
	a.videoMedia = vm
	a.useUDP = true
	a.mu.Unlock()
	a.touchActivity()
	a.cancelUDPSessionStale()
	audit.Log("noise_ok", "udp")
	i18n.Log("log.re2_noise_udp", a.id.DeviceID)
	if a.udpEP != nil && a.udpEP.UsingDirect() {
		log.Printf("re2 media plane=UDP·LAN device=%s video_plane=%v", a.id.DeviceID, vm != nil)
	} else {
		log.Printf("re2 media plane=UDP·Relay device=%s video_plane=%v", a.id.DeviceID, vm != nil)
	}
	trayCloseQRWindow()
	return nil
}

type udpPendingTransport struct {
	first []byte
	ep    *reudp.Endpoint
}

func (t *udpPendingTransport) Send(msg []byte) error {
	// Unreliable + App-layer retransmit. PreferDirect reliable ACK path often
	// black-holes Noise msg2 (REHP1 works, "Noise 已在 UDP 上建立" never logs).
	return t.ep.SendUnreliable(msg)
}

func (t *udpPendingTransport) Recv() ([]byte, error) {
	if t.first != nil {
		m := t.first
		t.first = nil
		return m, nil
	}
	// Bound wait for msg3 — indefinite Recv left the read loop stuck after a
	// stale-session re-handshake while the App already fell back to WSS.
	if t.ep == nil {
		return nil, fmt.Errorf("reudp: nil endpoint during noise")
	}
	return t.ep.RecvTimeout(2 * time.Second)
}

func (a *Agent) sendTunnel(msgType byte, body []byte, reliable bool) error {
	a.re2SendMu.Lock()
	defer a.re2SendMu.Unlock()
	if a.re2Sess == nil {
		return errNoSession
	}
	a.mu.Lock()
	ep := a.udpEP
	useUDP := a.useUDP && ep != nil
	conn := a.re2Conn
	a.mu.Unlock()
	inner := re2.EncodeInner(msgType, body)
	// MUST size-check BEFORE Encrypt — Noise nonces are one-shot. Encrypting an
	// oversized agent-chat list then failing SendReliable permanently desynced
	// Agent→phone AEAD (UDP quality reopen: 0 kb/s after "reudp: payload too large").
	const noiseTag = 16
	if useUDP && len(inner)+noiseTag > reudp.MaxPayload {
		return reudp.ErrTooLarge
	}
	// Same class of bug: Encrypt then ErrCongested burns the Agent→phone nonce so
	// later FileList/ACK replies are undecryptable while App→Agent (and video plane)
	// keep working. Wait for cwnd BEFORE Encrypt.
	// Noise CipherState is strictly ordered on every path. Once desktop video
	// moved to its independent VideoMedia cipher there is no valid reason to put
	// Noise ciphertext on unreliable REUDP: one lost stats/chat response burns a
	// nonce and makes every later control/file reply fail authentication.
	needReliable := useUDP && ep != nil
	if needReliable {
		ok := false
		for i := 0; i < 2000; i++ {
			if ep.CanSend() {
				ok = true
				break
			}
			// Drop re2SendMu while spinning so video/other control can progress ACKs.
			a.re2SendMu.Unlock()
			time.Sleep(2 * time.Millisecond)
			a.re2SendMu.Lock()
			if a.re2Sess == nil {
				return errNoSession
			}
			a.mu.Lock()
			ep = a.udpEP
			useUDP = a.useUDP && ep != nil
			a.mu.Unlock()
			if !useUDP || ep == nil {
				needReliable = false
				break
			}
		}
		if needReliable && !ok {
			return reudp.ErrCongested
		}
	}
	ct, err := a.re2Sess.Encrypt(inner)
	if err != nil {
		return err
	}
	// Single media plane = Noise transport. Mixing WSS+UDP ciphertext desyncs nonces
	// and made HUD "UDP·LAN" while video still rode the relay (field: always WSS).
	// useUDP (Noise over REUDP, PreferDirect routes LAN) → UDP; else → WSS.
	if useUDP && ep != nil {
		// All Noise control is reliable on LAN and relay; VideoMedia owns the
		// best-effort high-rate channel.
		if err := ep.SendReliable(ct); err != nil {
			log.Printf("sendTunnel: SendReliable after Encrypt failed mt=0x%02x err=%v — Agent→App Noise may be desynced", msgType, err)
			return err
		}
		return nil
	}
	if conn != nil {
		return conn.WriteFrame(re2.Frame{Type: re2.TypeTunnel, RouteID: a.id.DeviceID, Payload: ct})
	}
	return errNoSession
}

var errNoSession = errString("session not ready")

type errString string

func (e errString) Error() string { return string(e) }

// encodeCeiling is the path-aware commercial encode limit.
// Relay is a datacenter symmetric pipe (hundreds of Mbps), not volunteer home
// uplink. Client pathBudget + ABR still settle on weak phone Wi‑Fi / host last-mile.
// Never upscale past capture.
func encodeCeiling(useUDP, lanDirect bool) (maxW, maxH, maxBR, maxFPS int) {
	switch {
	case useUDP && lanDirect:
		// UDP·LAN PreferDirect — up to 16K when capture can produce it.
		return 15360, 8640, 300000, 30
	case useUDP:
		// UDP·Relay (datacenter): same resolution ceiling; bitrate below LAN.
		return 15360, 8640, 200000, 30
	default:
		// WSS·Relay (datacenter): allow full 16K; ABR backs off on loss / HOL.
		return 15360, 8640, 150000, 30
	}
}

func clampEncode(w, h, br, fps, maxW, maxH, maxBR, maxFPS int) (int, int, int, int) {
	if maxW > 0 && w > maxW {
		w = maxW
	}
	if maxH > 0 && h > maxH {
		h = maxH
	}
	if maxBR > 0 && br > maxBR {
		br = maxBR
	}
	if maxFPS > 0 && fps > maxFPS {
		fps = maxFPS
	}
	return w, h, br, fps
}

func (a *Agent) openDesktop(data re2.OpenDesktopPayload) error {
	a.deskOpenMu.Lock()
	defer a.deskOpenMu.Unlock()
	log.Printf("desktop OPEN req display=%d max=%dx%d@%d %dkbps codec=%q plane=%d",
		data.DisplayID, data.MaxWidth, data.MaxHeight, data.FPS, data.BitrateKbps, data.Codec, data.VideoPlane)
	if err := a.checkDesktopAccess(data.Password); err != nil {
		log.Printf("desktop OPEN denied: %v", err)
		audit.Log("desktop_denied", err.Error())
		return err
	}
	if runtime.GOOS == "darwin" {
		p := desktop.CheckHostPermissions()
		if !p.ScreenRecording {
			log.Printf("desktop OPEN requesting screen recording permission")
			if !desktop.RequestScreenRecording() {
				log.Printf("desktop OPEN blocked: screen recording permission missing")
				return errString(i18n.T("err.perm_screen"))
			}
			// Some macOS releases apply ScreenCapture only after relaunch.
			if !desktop.CheckHostPermissions().ScreenRecording {
				return errString("screen recording permission granted; restart RunEverything to apply")
			}
		}
		if !p.Accessibility {
			i18n.Log("log.perm_ax_need")
		}
	}

	maxW, maxH := data.MaxWidth, data.MaxHeight
	if maxW <= 0 {
		maxW = 1280
	}
	if maxH <= 0 {
		maxH = 720
	}
	fps := data.FPS
	if fps <= 0 {
		fps = 15
	}
	bitrate := data.BitrateKbps
	if bitrate <= 0 {
		bitrate = 2500
	}
	a.mu.Lock()
	udpMedia := a.useUDP
	lanDirect := udpMedia && a.udpEP != nil && a.udpEP.UsingDirect()
	wantVideoPlane := udpMedia && data.VideoPlane >= 1 && a.videoMedia != nil
	a.mu.Unlock()
	cw, ch, cbr, cfps := encodeCeiling(udpMedia, lanDirect)
	maxW, maxH, bitrate, fps = clampEncode(maxW, maxH, bitrate, fps, cw, ch, cbr, cfps)
	// VT hard-max is 8192; full-blood 16K uses libx265 (~2–4s/frame). Cap FPS +
	// bitrate BEFORE soft-reopen so a second OPEN cannot undo pacing and blast a
	// 10k+ part IDR that PreferDirect never finishes assembling.
	if maxW > 8192 || maxH > 8192 {
		if fps > 2 {
			fps = 2
		}
		if cfps > 2 {
			cfps = 2
		}
		// ~12 Mbps @ 2fps keeps Annex-B IDRs in the low-MB range (≤~2–3k UDP parts).
		if bitrate > 12000 {
			bitrate = 12000
		}
	} else if maxW >= 7680 || maxH >= 4320 {
		if fps > 5 {
			fps = 5
		}
		if cfps > 5 {
			cfps = 5
		}
	}

	// Same session + smaller/equal encode: hot-reconfigure ABR/encoder. Full
	// closeDesktop()+SCK restart mid-stream was peer_gone on PreferDirect LAN.
	a.mu.Lock()
	softOK := a.deskCancel != nil && a.deskEnc != nil && a.deskABR != nil &&
		a.deskSID != "" && a.deskSID == data.SessionID &&
		a.deskCapMaxW > 0 && maxW <= a.deskCapMaxW && maxH <= a.deskCapMaxH
	a.mu.Unlock()
	if softOK {
		return a.reopenDesktopSoft(data, maxW, maxH, bitrate, fps, wantVideoPlane)
	}

	a.closeDesktop()
	desktop.SetSelectedMonitor(data.DisplayID)
	desktop.SetHideCursor(data.HideCursor)
	if data.PrivacyBlank {
		_ = desktop.SetPrivacyBlank(true)
	} else {
		_ = desktop.SetPrivacyBlank(false)
	}

	wantHEVC := strings.EqualFold(data.Codec, "h265") || strings.EqualFold(data.Codec, "hevc") ||
		maxW >= 3840 || maxH >= 2160
	// Inline SCK→VT for HEVC at ≥720p (avoids Go full-panel copies on the 5K secondary).
	// Above VT 8192, inline create is skipped anyway — don't even request it so
	// capture targets virtual FB pixels + libx265.
	// Keep ordinary ≤desktop-native sessions on the pixel path: it can replay a
	// cached frame when SCK is idle, which makes quality/background reopens paint
	// even on a completely static desktop. Inline HEVC remains for true ≥4K where
	// avoiding full-panel Go copies is essential.
	inlineHEVC := wantHEVC && (maxW >= 3840 || maxH >= 2160) && maxW <= 8192 && maxH <= 8192
	if inlineHEVC {
		desktop.SetCaptureInlineHEVC(maxW, maxH, bitrate, fps)
	}
	cap, err := deskbridge.NewCapturer()
	if err != nil {
		desktop.ClearCaptureInlineHEVC()
		log.Printf("desktop: capturer failed %dx%d: %v", maxW, maxH, err)
		return fmt.Errorf("desktop capturer %dx%d: %w", maxW, maxH, err)
	}
	inj, err := deskbridge.NewInjector()
	if err != nil {
		_ = cap.Close()
		log.Printf("desktop: injector failed: %v", err)
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	log.Printf("desktop capture starting %dx%d", maxW, maxH)
	frames, err := cap.Start(ctx, maxW, maxH)
	if err != nil {
		cancel()
		_ = cap.Close()
		_ = inj.Close()
		log.Printf("desktop: capture Start failed: %v", err)
		return err
	}
	var first desktop.Frame
	firstWait := 8 * time.Second
	if maxW > 8192 || maxH > 8192 {
		firstWait = 90 * time.Second
	} else if maxW >= 7680 || maxH >= 4320 {
		firstWait = 25 * time.Second
	}
	select {
	case f, ok := <-frames:
		if !ok || (f.Img == nil && len(f.AnnexB) == 0) {
			cancel()
			_ = cap.Close()
			_ = inj.Close()
			log.Printf("desktop: capture ended without frame")
			return errString("desktop capture ended without frame")
		}
		first = f
		log.Printf("desktop capture first frame ok")
	case <-time.After(firstWait):
		cancel()
		_ = cap.Close()
		_ = inj.Close()
		log.Printf("desktop: capture timeout waiting first frame after %s (%dx%d)", firstWait, maxW, maxH)
		return errString("desktop capture timeout")
	}
	var w, h int
	if first.Img != nil {
		w, h = first.Img.Bounds().Dx(), first.Img.Bounds().Dy()
		// Honor client max size so the first IDR does not explode into 60+ UDP parts.
		if maxW > 0 && w > maxW || maxH > 0 && h > maxH {
			tw, th := w, h
			if maxW > 0 && tw > maxW {
				tw = maxW
			}
			if maxH > 0 && th > maxH {
				th = maxH
			}
			first.Img = desktop.ScaleExact(first.Img, tw, th)
			w, h = tw, th
		}
	} else {
		w, h = cap.Size()
		if w <= 0 {
			w = maxW
		}
		if h <= 0 {
			h = maxH
		}
	}
	var enc desktop.Encoder
	codecName := desktop.CodecH264
	if inlineHEVC && len(first.AnnexB) > 0 {
		enc = &inlineHEVCEncoder{w: w, h: h}
		codecName = desktop.CodecH265
		log.Printf("desktop inline HEVC capture %dx%d@%d %dkbps", w, h, fps, bitrate)
	} else {
		enc, err = desktop.NewEncoderBitrateCodec(w, h, fps, bitrate, wantHEVC)
		if err != nil {
			cancel()
			_ = cap.Close()
			_ = inj.Close()
			return err
		}
		if namer, ok := enc.(desktop.CodecNamer); ok {
			codecName = namer.CodecName()
		}
		if wantHEVC && codecName != desktop.CodecH265 {
			log.Printf("desktop HEVC requested but encoder fell back to %s %dx%d", codecName, w, h)
		}
	}
	if di, ok := inj.(interface{ SetScreenSize(int, int) }); ok {
		di.SetScreenSize(w, h)
	}
	abrCfg := desktop.DefaultABR()
	// Cap ABR to the client's OPEN limits — otherwise healthy STATS ramp past
	// the negotiated quality rung.
	if maxW > 0 {
		abrCfg.MaxWidth = maxW
	}
	if maxH > 0 {
		abrCfg.MaxHeight = maxH
	}
	if fps > 0 {
		abrCfg.MaxFPS = fps
	}
	if bitrate > 0 {
		abrCfg.MaxBitrateK = bitrate
		if abrCfg.MinBitrateK > bitrate {
			abrCfg.MinBitrateK = max(200, bitrate/2)
		}
	}
	// Native display = ladder reference (超清=1.0 … 流畅=0.35). OPEN max only
	// caps the top rung — never invent 0.35×标清 mid-sizes.
	if mons, err := desktop.ListMonitors(); err == nil {
		for _, m := range mons {
			if data.DisplayID != 0 && m.ID != data.DisplayID {
				continue
			}
			nw, nh := m.Width, m.Height
			if desktop.IsVirtualDisplay(m.ID) {
				if fw, fh, ok := desktop.VirtualFramebuffer(m.ID); ok {
					nw, nh = fw, fh
				}
			}
			if nw > 0 && nh > 0 {
				abrCfg.NativeWidth, abrCfg.NativeHeight = nw, nh
				break
			}
		}
		if abrCfg.NativeWidth <= 0 {
			for _, m := range mons {
				if !m.Primary {
					continue
				}
				nw, nh := m.Width, m.Height
				if desktop.IsVirtualDisplay(m.ID) {
					if fw, fh, ok := desktop.VirtualFramebuffer(m.ID); ok {
						nw, nh = fw, fh
					}
				}
				if nw > 0 && nh > 0 {
					abrCfg.NativeWidth, abrCfg.NativeHeight = nw, nh
				}
				break
			}
		}
	}
	// Floor = 流畅 at 0.35×native (or OPEN when native unknown).
	refW, refH := abrCfg.NativeWidth, abrCfg.NativeHeight
	if refW <= 0 {
		refW = abrCfg.MaxWidth
	}
	if refH <= 0 {
		refH = abrCfg.MaxHeight
	}
	if refW > 0 && refH > 0 {
		abrCfg.MinWidth = max(2, int(float64(refW)*0.35+0.5)) &^ 1
		abrCfg.MinHeight = max(2, int(float64(refH)*0.35+0.5)) &^ 1
		if abrCfg.MinWidth > abrCfg.MaxWidth {
			abrCfg.MinWidth = abrCfg.MaxWidth
		}
		if abrCfg.MinHeight > abrCfg.MaxHeight {
			abrCfg.MinHeight = abrCfg.MaxHeight
		}
	}
	// WSS soft-cap: ordered Noise tunnel ceiling = 4K (3840×2160).
	// Higher OPEN (5K/8K) is scaled down on WSS; UDP·Relay / PreferDirect uncapped here.
	const wssSoftMaxW, wssSoftMaxH = 3840, 2160
	if !udpMedia {
		if abrCfg.MaxWidth <= 0 || abrCfg.MaxWidth > wssSoftMaxW {
			abrCfg.MaxWidth = wssSoftMaxW
		}
		if abrCfg.MaxHeight <= 0 || abrCfg.MaxHeight > wssSoftMaxH {
			abrCfg.MaxHeight = wssSoftMaxH
		}
		abrCfg.MinWidth = max(2, int(float64(abrCfg.MaxWidth)*0.35+0.5)) &^ 1
		abrCfg.MinHeight = max(2, int(float64(abrCfg.MaxHeight)*0.35+0.5)) &^ 1
	}
	abrW, abrH := w, h
	if abrCfg.MaxWidth > 0 && abrW > abrCfg.MaxWidth {
		abrW = abrCfg.MaxWidth
	}
	if abrCfg.MaxHeight > 0 && abrH > abrCfg.MaxHeight {
		abrH = abrCfg.MaxHeight
	}
	if abrW > 0 && abrH > 0 && (abrW != w || abrH != h) {
		if first.Img != nil {
			first.Img = desktop.ScaleExact(first.Img, abrW, abrH)
		}
		w, h = abrW, abrH
		if di, ok := inj.(interface{ SetScreenSize(int, int) }); ok {
			di.SetScreenSize(w, h)
		}
		// Rebuild encoder at capped size when capture opened larger than WSS soft cap.
		if !inlineHEVC || len(first.AnnexB) == 0 {
			if enc != nil {
				_ = enc.Close()
			}
			enc, err = desktop.NewEncoderBitrateCodec(w, h, fps, bitrate, wantHEVC)
			if err != nil {
				cancel()
				_ = cap.Close()
				_ = inj.Close()
				return err
			}
			if namer, ok := enc.(desktop.CodecNamer); ok {
				codecName = namer.CodecName()
			}
		}
	}
	abr := desktop.NewABR(abrCfg, w, h, fps, bitrate)
	// READY + first IDR must match ABR encode size (NewABR may snap to a
	// ladder rung that differs slightly from capture after clamp).
	if aw, ah, _, _, _ := abr.Snapshot(); aw > 0 && ah > 0 && (aw != w || ah != h) {
		if first.Img != nil {
			first.Img = desktop.ScaleExact(first.Img, aw, ah)
		}
		w, h = aw, ah
		if di, ok := inj.(interface{ SetScreenSize(int, int) }); ok {
			di.SetScreenSize(w, h)
		}
		if !inlineHEVC || len(first.AnnexB) == 0 {
			if enc != nil {
				_ = enc.Close()
			}
			enc, err = desktop.NewEncoderBitrateCodec(w, h, fps, bitrate, wantHEVC)
			if err != nil {
				cancel()
				_ = cap.Close()
				_ = inj.Close()
				return err
			}
			if namer, ok := enc.(desktop.CodecNamer); ok {
				codecName = namer.CodecName()
			}
		}
	}
	abr.RequestKeyframe()
	mons, _ := desktop.ListMonitors()
	dinfos := make([]re2.DisplayInfo, 0, len(mons))
	for _, m := range mons {
		di := re2.DisplayInfo{ID: m.ID, Name: m.Name, Width: m.Width, Height: m.Height, X: m.X, Y: m.Y, Primary: m.Primary}
		if desktop.IsVirtualDisplay(m.ID) {
			di.Virtual = true
			if fw, fh, ok := desktop.VirtualFramebuffer(m.ID); ok {
				di.FBWidth, di.FBHeight = fw, fh
			}
		}
		dinfos = append(dinfos, di)
	}

	a.mu.Lock()
	a.deskCancel = cancel
	a.deskCap = cap
	a.deskInj = inj
	a.deskEnc = enc
	a.deskEncW = w
	a.deskEncH = h
	a.deskSID = data.SessionID
	a.deskABR = abr
	a.deskCapMaxW = maxW
	a.deskCapMaxH = maxH
	a.deskVideoPlane = wantVideoPlane
	a.inputCh = make(chan desktopInputEvent, 128)
	a.inputNextID = 0 // latch the first client event ID for this desktop generation
	a.inputPending = make(map[uint64]desktopInputEvent)
	inputCh := a.inputCh
	a.noteControlPeerLocked()
	notify := a.onDesktopChange
	a.mu.Unlock()
	a.touchActivity()
	audit.Log("desktop_open", data.SessionID)
	if wantVideoPlane {
		log.Printf("desktop video_plane=v1 session=%s %dx%d@%d %dkbps lan=%v",
			data.SessionID, w, h, fps, bitrate, lanDirect)
	}
	if notify != nil {
		notify(true)
	}

	go a.desktopInputPump(ctx, inputCh)
	go a.desktopPump(frames, first, data.SessionID, fps)
	go a.cursorPump(data.SessionID, ctx)
	go a.audioPump(data.SessionID, ctx)
	go a.clipboardPump(data.SessionID, ctx)
	vp := 0
	if wantVideoPlane {
		vp = 1
	}
	ready := re2.DesktopReadyPayload{
		SessionID:   data.SessionID,
		Width:       w,
		Height:      h,
		Codec:       codecName,
		FPS:         fps,
		BitrateKbps: bitrate,
		DisplayID:   data.DisplayID,
		Displays:    dinfos,
		VideoPlane:  vp,
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		p := desktop.CheckHostPermissions()
		mic, cam := p.Microphone, p.Camera
		ready.MicAuthorized = &mic
		ready.CameraAuthorized = &cam
	}
	body := re2.MustJSON(ready)
	// REUDP MaxPayload≈1200; a fat virtual-display list can exceed it and used to
	// Encrypt-then-fail (nonce burn). Trim displays until the frame fits.
	for len(dinfos) > 0 && a.useUDP && len(body)+16 > reudp.MaxPayload {
		dinfos = dinfos[:len(dinfos)-1]
		ready.Displays = dinfos
		body = re2.MustJSON(ready)
		log.Printf("desktop READY trimmed displays→%d for REUDP", len(dinfos))
	}
	return a.sendTunnel(re2.MsgDesktopReady, body, true)
}

func (a *Agent) checkDesktopAccess(password string) error {
	want := strings.TrimSpace(os.Getenv("RE_ACCESS_PASSWORD"))
	if want != "" && password != want {
		return errString("access password required")
	}
	confirm := os.Getenv("RE_PAIR_CONFIRM")
	if confirm == "1" || confirm == "true" || confirm == "yes" {
		if !desktop.ConfirmLocal(i18n.T("desktop.confirm"), 60*time.Second) {
			return errString("remote desktop denied on agent")
		}
	}
	return nil
}

func (a *Agent) cursorPump(sid string, ctx context.Context) {
	cr := desktop.NewCursorReader()
	t := time.NewTicker(33 * time.Millisecond) // ~30Hz when moving
	defer t.Stop()
	var lastX, lastY float64 = -1, -1
	var lastSent time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pos, err := cr.Cursor()
			if err != nil {
				continue
			}
			moved := lastX < 0 ||
				absFloat(pos.X-lastX) > 0.0008 ||
				absFloat(pos.Y-lastY) > 0.0008
			// Heartbeat so client doesn't think the cursor died; skip spam when idle
			// so unreliable CURSOR doesn't fight fat H.264 parts on PreferDirect.
			if !moved && time.Since(lastSent) < 400*time.Millisecond {
				continue
			}
			lastX, lastY = pos.X, pos.Y
			lastSent = time.Now()
			// Always advertise visible — phone draws its own overlay; host OS
			// cursor visibility is unrelated (and often false while typing).
			_ = a.sendTunnel(re2.MsgCursor, re2.MustJSON(re2.CursorPayload{
				SessionID: sid, X: pos.X, Y: pos.Y, Visible: true,
			}), false)
		}
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func (a *Agent) audioPump(sid string, ctx context.Context) {
	ac, err := desktop.StartAudioCapture(ctx, 48000)
	if err != nil || ac == nil {
		return
	}
	defer ac.Close()
	codec := ac.Codec()
	sr := ac.SampleRate()
	// ~20ms @ 48k mono s16 = 1920 bytes; opus reads variable
	chunk := 1920
	if codec == "opus" {
		chunk = 1024
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
			pcm, err := ac.ReadPCM(chunk)
			if err != nil || len(pcm) == 0 {
				time.Sleep(desktop.AudioPumpInterval)
				continue
			}
			_ = a.sendTunnel(re2.MsgAudio, re2.MustJSON(re2.AudioPayload{
				SessionID: sid, Codec: codec, SampleRate: sr, Channels: 1,
				DataB64: desktop.EncodeAudioPCM16(pcm),
			}), false)
		}
	}
}

func (a *Agent) clipboardPump(sid string, ctx context.Context) {
	if a.clip == nil {
		a.clip = desktop.NewClipboardHub()
	}
	lastText := a.clip.GetText()
	var lastPNGLen int
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cur := a.clip.GetText()
			if cur != "" && cur != lastText {
				lastText = cur
				_ = a.sendTunnel(re2.MsgClipboard, re2.MustJSON(re2.ClipboardPayload{
					SessionID: sid, Mime: "text/plain", Text: cur,
				}), false)
			}
			if png := a.clip.GetPNG(); len(png) > 0 && len(png) != lastPNGLen {
				lastPNGLen = len(png)
				_ = a.sendTunnel(re2.MsgClipboard, re2.MustJSON(re2.ClipboardPayload{
					SessionID: sid, Mime: "image/png", DataB64: desktop.EncodePNGB64(png),
				}), false)
			}
		}
	}
}

func (a *Agent) desktopPump(frames <-chan desktop.Frame, first desktop.Frame, sid string, fps int) {
	// PreferDirect LAN skips 2× IDR mirror after the first keyframe (encode loop
	// saturation). The first IDR after OPEN — especially post iOS app-switch —
	// still needs one reverse pass or the client sits on "waiting for video".
	lanFirstIDRMirrored := false
	sendFrame := func(f desktop.Frame, key bool) {
		var readyAfterKey []byte
		resizedNow := false
		a.mu.Lock()
		// Apply soft-reopen resize on this goroutine only — Close+create here
		// cannot race an in-flight Encode.
		if a.deskResizePending {
			rw, rh, rfps, rbr := a.deskResizeW, a.deskResizeH, a.deskResizeFPS, a.deskResizeBR
			wantHEVC := a.deskResizeHEVC
			a.deskResizePending = false
			encCur := a.deskEnc
			a.mu.Unlock()
			reconfigured := false
			if rc, ok := encCur.(desktop.Reconfigurer); ok {
				if err := rc.Reconfigure(rw, rh, rfps, rbr); err != nil {
					log.Printf("desktop soft resize Reconfigure %dx%d: %v — recreate", rw, rh, err)
				} else {
					reconfigured = true
					a.mu.Lock()
					a.deskEncW, a.deskEncH = rw, rh
					a.deskForceKeyUntil = time.Now().Add(5 * time.Second)
					a.mu.Unlock()
					log.Printf("desktop soft resize reconfigured %dx%d@%d %dkbps", rw, rh, rfps, rbr)
				}
			}
			if !reconfigured {
				if encCur != nil {
					_ = encCur.Close()
				}
				newEnc, err := desktop.NewEncoderBitrateCodec(rw, rh, rfps, rbr, wantHEVC)
				a.mu.Lock()
				if err != nil {
					log.Printf("desktop soft resize encoder %dx%d: %v", rw, rh, err)
					a.deskEnc = nil
				} else {
					a.deskEnc = newEnc
					a.deskEncW, a.deskEncH = rw, rh
					a.deskForceKeyUntil = time.Now().Add(5 * time.Second)
					resizedNow = true
					log.Printf("desktop soft resize encoder ready %dx%d@%d %dkbps", rw, rh, rfps, rbr)
				}
				a.mu.Unlock()
			} else {
				resizedNow = true
			}
			a.deskForceKey.Store(true)
			a.mu.Lock()
		}
		// Flush deferred READY after this frame if we emit a key.
		if len(a.deskReadyPending) > 0 {
			readyAfterKey = a.deskReadyPending
			// Keep pending until a key is actually sent; cleared below on success.
		}
		enc := a.deskEnc
		sess := a.re2Sess
		ep := a.udpEP
		useUDP := a.useUDP
		conn := a.re2Conn
		abr := a.deskABR
		vm := a.videoMedia
		videoPlane := a.deskVideoPlane && vm != nil && useUDP && ep != nil
		deviceID := a.id.DeviceID
		forceUntil := a.deskForceKeyUntil
		lastSoftKey := a.deskLastSoftKeyAt
		a.mu.Unlock()
		if enc == nil || (f.Img == nil && len(f.AnnexB) == 0) {
			return
		}
		if !videoPlane && sess == nil {
			return
		}
		forceKey := key
		if resizedNow || len(readyAfterKey) > 0 {
			forceKey = true
		} else if time.Now().Before(forceUntil) {
			// Keep pulling IDRs after soft reopen, but never faster than ~1.2Hz —
			// per-frame keys tore multipart assembly on PreferDirect / Relay.
			if time.Since(lastSoftKey) >= 800*time.Millisecond {
				forceKey = true
			}
		}
		// Send deferred READY *before* this IDR. IDR-then-READY raced the
		// video_plane vs control path: client reset VT after consuming the
		// only new-SPS keyframe → pic stuck at old size, RX dropped to 0 kb/s.
		if len(readyAfterKey) > 0 {
			a.mu.Lock()
			if len(a.deskReadyPending) > 0 {
				a.deskReadyPending = nil
			}
			a.deskForceKeyUntil = time.Now().Add(5 * time.Second)
			a.mu.Unlock()
			a.deskForceKey.Store(true)
			a.touchActivity()
			if err := a.sendTunnel(re2.MsgDesktopReady, readyAfterKey, true); err != nil {
				log.Printf("desktop soft resize READY send: %v", err)
				a.mu.Lock()
				a.deskReadyPending = readyAfterKey
				a.mu.Unlock()
			} else {
				log.Printf("desktop soft reopen READY sent before IDR")
				readyAfterKey = nil
			}
			forceKey = true
		}
		inline := len(f.AnnexB) > 0
		if abr != nil && f.Img != nil && !inline {
			tw, th, tfps, tbr, fk := abr.Snapshot()
			if fk {
				forceKey = true
			}
			srcW, srcH := f.Img.Bounds().Dx(), f.Img.Bounds().Dy()
			if tw > srcW {
				tw = srcW
			}
			if th > srcH {
				th = srcH
			}
			lanDirect := ep != nil && ep.UsingDirect()
			cw, ch, cbr, cfps := encodeCeiling(useUDP, lanDirect)
			tw, th, tbr, tfps = clampEncode(tw, th, tbr, tfps, cw, ch, cbr, cfps)
			// WSS-only soft cap (ordered Noise tunnel): ≤4K.
			if !videoPlane && !useUDP {
				if tw > 3840 {
					tw = 3840
				}
				if th > 2160 {
					th = 2160
				}
				if forceKey && tbr > 25000 {
					tbr = 25000
				}
			}
			if tw > 0 && th > 0 && (srcW != tw || srcH != th) {
				f.Img = desktop.ScaleExact(f.Img, tw, th)
			}
			if rc, ok := enc.(desktop.Reconfigurer); ok {
				prevW, prevH := 0, 0
				a.mu.Lock()
				prevW, prevH = a.deskEncW, a.deskEncH
				a.mu.Unlock()
				_ = rc.Reconfigure(tw, th, tfps, tbr)
				if tw != prevW || th != prevH {
					a.mu.Lock()
					a.deskEncW, a.deskEncH = tw, th
					a.mu.Unlock()
				}
			}
			_ = cfps
		} else if forceKey && f.Img != nil && !inline {
			srcW, srcH := f.Img.Bounds().Dx(), f.Img.Bounds().Dy()
			tw, th := srcW, srcH
			lanDirect := ep != nil && ep.UsingDirect()
			cw, ch, _, _ := encodeCeiling(useUDP, lanDirect)
			if tw > cw {
				tw = cw
			}
			if th > ch {
				th = ch
			}
			if tw != srcW || th != srcH {
				f.Img = desktop.ScaleExact(f.Img, tw, th)
			}
		}
		var annexB []byte
		var err error
		if inline {
			annexB = f.AnnexB
			// Wire key flag must match the bitstream — never mark a P-frame as IDR
			// (was breaking iOS HEVC: tiny "keys" without VPS/SPS/PPS).
			forceKey = f.Keyframe
			if key && !f.Keyframe {
				if rk, ok := a.deskCap.(interface{ RequestKeyframe() }); ok {
					rk.RequestKeyframe()
				}
			}
		} else {
			annexB, err = enc.Encode(f, forceKey)
		}
		if err != nil || len(annexB) == 0 {
			if forceKey {
				// Soft reopen / size change must not burn the forced IDR on a
				// transient empty VT output — otherwise pic stays at the old SPS.
				a.deskForceKey.Store(true)
				log.Printf("video encode failed key=1 err=%v len=%d — retry force key", err, len(annexB))
			}
			return
		}
		// IDs describe frames actually put on the wire. VideoToolbox may return an
		// empty output while buffering/throttling; consuming an ID before Encode
		// made the client report those local skips as 30–60% network loss.
		a.mu.Lock()
		fid := a.deskFrameID
		a.deskFrameID++
		a.mu.Unlock()
		if forceKey {
			a.mu.Lock()
			a.deskLastSoftKeyAt = time.Now()
			a.mu.Unlock()
		}
		// Note: do not re-encode/shrink forced IDRs — VT often emits invalid
		// bitstreams under aggressive bitrate/quality clamps (iOS -12909 BadData).
		flags := byte(0)
		if forceKey {
			flags = re2.VideoFlagKeyFrame
		}
		chunkBudget := reudp.MaxPayload - 96
		if videoPlane {
			chunkBudget = re2.MaxVideoPlainUDP() - 8
		}
		parts := re2.FragmentNAL(sid, fid, flags, annexB, chunkBudget)
		if forceKey {
			cached := make([][]byte, len(parts))
			for i := range parts {
				cached[i] = append([]byte(nil), parts[i]...)
			}
			a.mu.Lock()
			if a.deskKeyFrames == nil {
				a.deskKeyFrames = make(map[uint32]desktopKeyframeCache)
			}
			a.deskKeyFrames[fid] = desktopKeyframeCache{parts: cached, at: time.Now()}
			a.deskKeyOrder = append(a.deskKeyOrder, fid)
			for len(a.deskKeyOrder) > 4 {
				oldest := a.deskKeyOrder[0]
				a.deskKeyOrder = a.deskKeyOrder[1:]
				delete(a.deskKeyFrames, oldest)
			}
			a.mu.Unlock()
			i18n.Log("log.video_keyframe", fid, len(parts), len(annexB))
		}
		sendVideoPart := func(pi int, part []byte) bool {
			inner := re2.EncodeInner(re2.MsgVideo, part)
			if videoPlane {
				ct, err := vm.Seal(inner)
				if err != nil {
					log.Printf("video plane seal frame=%d part=%d/%d: %v", fid, pi, len(parts), err)
					return false
				}
				if len(ct) > reudp.MaxPayload {
					log.Printf("video plane part too large frame=%d part=%d/%d ct=%d",
						fid, pi, len(parts), len(ct))
					return false
				}
				if err := ep.SendUnreliable(ct); err != nil {
					log.Printf("video plane send frame=%d part=%d/%d: %v", fid, pi, len(parts), err)
					return false
				}
				return true
			}
			// Size-check BEFORE Encrypt — burning a Noise nonce on oversized parts
			// permanently desyncs Agent→phone (same class of bug as agent-chat list).
			const noiseTag = 16
			if len(inner)+noiseTag > reudp.MaxPayload {
				log.Printf("video part too large before encrypt frame=%d part=%d/%d plain=%d",
					fid, pi, len(parts), len(inner)+noiseTag)
				return false
			}
			a.re2SendMu.Lock()
			ct, err := sess.Encrypt(inner)
			if err != nil {
				a.re2SendMu.Unlock()
				log.Printf("video encrypt frame=%d part=%d/%d: %v", fid, pi, len(parts), err)
				return false
			}
			if len(ct) > reudp.MaxPayload {
				a.re2SendMu.Unlock()
				log.Printf("video part too large after encrypt frame=%d part=%d/%d ct=%d (>%d)",
					fid, pi, len(parts), len(ct), reudp.MaxPayload)
				return false
			}
			var sendErr error
			if useUDP && ep != nil {
				for retry := 0; retry < 2000; retry++ {
					sendErr = ep.SendReliable(ct)
					if sendErr == nil || sendErr != reudp.ErrCongested {
						break
					}
					a.re2SendMu.Unlock()
					time.Sleep(2 * time.Millisecond)
					a.re2SendMu.Lock()
				}
			} else if conn != nil {
				sendErr = conn.WriteFrame(re2.Frame{Type: re2.TypeTunnel, RouteID: deviceID, Payload: ct})
			}
			a.re2SendMu.Unlock()
			if sendErr != nil {
				log.Printf("video send frame=%d part=%d/%d: %v", fid, pi, len(parts), sendErr)
				return false
			}
			return true
		}
		pace := forceKey && videoPlane && len(parts) > 32
		paceEvery := 1 // sleep after every N parts
		paceSleep := 200 * time.Microsecond
		if len(parts) > 80 {
			paceSleep = 800 * time.Microsecond
		}
		if len(parts) > 120 {
			paceSleep = 1500 * time.Microsecond
		}
		for pi, part := range parts {
			if !sendVideoPart(pi, part) {
				return
			}
			if pace && (pi+1)%paceEvery == 0 {
				time.Sleep(paceSleep)
			}
		}
		// Soft-reopen IDR: PreferDirect video_plane has been dropping to 0 kb/s on
		// the sim after READY (Agent still mirrors 672p keys). Also put this IDR
		// on Noise reliable so the client can rebuild SPS even if media AEAD/UDP
		// stalls. Assembler keys by frame+part — duplicates are free.
		// Skip when cwnd is tight — Encrypt-then-Congested permanently desyncs
		// Agent→App Noise (FileList/ACK replies become undecryptable).
		if forceKey && videoPlane && sess != nil && time.Now().Before(forceUntil.Add(2*time.Second)) && len(parts) <= 48 {
			reliableOK := 0
			for _, part := range parts {
				if ep != nil && !ep.CanSend() {
					break
				}
				inner := re2.EncodeInner(re2.MsgVideo, part)
				const noiseTag = 16
				if len(inner)+noiseTag > reudp.MaxPayload {
					break
				}
				a.re2SendMu.Lock()
				ct, err := sess.Encrypt(inner)
				if err != nil {
					a.re2SendMu.Unlock()
					break
				}
				var sendErr error
				if ep != nil {
					sendErr = ep.SendReliable(ct)
				} else if conn != nil {
					sendErr = conn.WriteFrame(re2.Frame{Type: re2.TypeTunnel, RouteID: deviceID, Payload: ct})
				}
				a.re2SendMu.Unlock()
				if sendErr != nil {
					log.Printf("video soft-reopen reliable abort after Encrypt frame=%d err=%v — stop to avoid Noise desync", fid, sendErr)
					break
				}
				reliableOK++
			}
			if reliableOK > 0 {
				log.Printf("video soft-reopen IDR also on reliable frame=%d parts=%d/%d", fid, reliableOK, len(parts))
			}
		}
		// Single-path keyframe redundancy on lossy video plane (same Wi‑Fi / one NIC).
		// Client VideoFrameAssembler keys by part index, so duplicates are free.
		// Reverse second pass survives contiguous burst drops better than back-to-back
		// doubles. Skip when parts==1 or non-key / reliable path.
		// On PreferDirect LAN, fat 5K IDRs (~250+ parts) must not be mirrored —
		// 2× send was saturating the encode loop at ~1fps.
		lanDirect := ep != nil && ep.UsingDirect()
		if forceKey && videoPlane && len(parts) > 1 && !(lanDirect && len(parts) > 96) {
			for i := len(parts) - 1; i >= 0; i-- {
				if !sendVideoPart(i, parts[i]) {
					return
				}
				if pace {
					time.Sleep(paceSleep)
				}
			}
			// Sparse third pass on fat IDRs — fill random holes (skip on LAN).
			if !lanDirect && len(parts) > 64 {
				for i := 0; i < len(parts); i += 3 {
					if !sendVideoPart(i, parts[i]) {
						return
					}
					if pace {
						time.Sleep(paceSleep / 2)
					}
				}
			}
			srcW, srcH := 0, 0
			if f.Img != nil {
				srcW, srcH = f.Img.Bounds().Dx(), f.Img.Bounds().Dy()
			}
			log.Printf("video keyframe mirror frame=%d parts=%d annexB=%d enc=%dx%d (single-path 2x+)",
				fid, len(parts), len(annexB), srcW, srcH)
		} else if forceKey && videoPlane && lanDirect && len(parts) > 96 {
			srcW, srcH := 0, 0
			if f.Img != nil {
				srcW, srcH = f.Img.Bounds().Dx(), f.Img.Bounds().Dy()
			}
			if !lanFirstIDRMirrored {
				lanFirstIDRMirrored = true
				for i := len(parts) - 1; i >= 0; i-- {
					if !sendVideoPart(i, parts[i]) {
						return
					}
					if pace {
						time.Sleep(paceSleep)
					}
				}
				log.Printf("video keyframe LAN first-IDR mirror frame=%d parts=%d annexB=%d enc=%dx%d",
					fid, len(parts), len(annexB), srcW, srcH)
			} else {
				log.Printf("video keyframe LAN skip-mirror frame=%d parts=%d annexB=%d enc=%dx%d",
					fid, len(parts), len(annexB), srcW, srcH)
			}
		}
	}
	sendFrame(first, true)
	n := 0
	lastKeyAt := time.Now()
	for f := range frames {
		n++
		// Frame-based keys for normal FPS; wall-clock fallback when 5K encode
		// drops to ~1fps (keyEvery=150 frames used to mean minutes between IDRs).
		keyEvery := 30
		keyMaxGap := 5 * time.Second
		a.mu.Lock()
		abr := a.deskABR
		a.mu.Unlock()
		if abr != nil {
			aw, ah, _, _, _ := abr.Snapshot()
			if aw >= 3840 || ah >= 2160 {
				// HEVC 5K: fewer fat IDRs; P-frames carry motion once encode is real-time.
				keyEvery = 120
				keyMaxGap = 12 * time.Second
			} else if aw >= 2560 || ah >= 1440 {
				// Prefer P-frames for smoothness; fat IDRs every ~8s are enough
				// for Wi‑Fi recovery on 1440p secondary Ultra.
				keyEvery = 75
				keyMaxGap = 8 * time.Second
			}
		}
		forceKey := n%keyEvery == 0
		if !forceKey && time.Since(lastKeyAt) >= keyMaxGap {
			forceKey = true
		}
		// Hard debounce — never IDR-storm (was firing every 2s and breaking OPEN).
		// WSS multipart IDRs need longer gaps or the client never finishes a frame.
		a.mu.Lock()
		udpMedia := a.useUDP
		a.mu.Unlock()
		minKeyGap := 800 * time.Millisecond
		if !udpMedia {
			minKeyGap = 2200 * time.Millisecond
		}
		if forceKey && time.Since(lastKeyAt) < minKeyGap {
			forceKey = false
		}
		if a.deskForceKey.CompareAndSwap(true, false) {
			forceKey = true
		}
		if forceKey {
			lastKeyAt = time.Now()
		}
		sendFrame(f, forceKey)
		a.mu.Lock()
		alive := a.deskSID == sid
		a.mu.Unlock()
		if !alive {
			return
		}
	}
}

func (a *Agent) reopenDesktopSoft(data re2.OpenDesktopPayload, maxW, maxH, bitrate, fps int, wantVideoPlane bool) error {
	desktop.SetHideCursor(data.HideCursor)
	if data.PrivacyBlank {
		_ = desktop.SetPrivacyBlank(true)
	} else {
		_ = desktop.SetPrivacyBlank(false)
	}
	a.mu.Lock()
	enc := a.deskEnc
	abr := a.deskABR
	cap := a.deskCap
	udpMedia := a.useUDP
	lanDirect := udpMedia && a.udpEP != nil && a.udpEP.UsingDirect()
	a.deskVideoPlane = wantVideoPlane
	a.mu.Unlock()
	if enc == nil || abr == nil {
		return errString("desktop soft reopen: not active")
	}
	abrCfg := desktop.DefaultABR()
	abrCfg.MaxWidth, abrCfg.MaxHeight = maxW, maxH
	abrCfg.MaxBitrateK, abrCfg.MaxFPS = bitrate, fps
	if m, ok := desktop.SelectedMonitorInfo(); ok && m.Width > 0 && m.Height > 0 {
		abrCfg.NativeWidth, abrCfg.NativeHeight = m.Width, m.Height
	}
	if !udpMedia {
		const wssSoftMaxW, wssSoftMaxH = 3840, 2160
		if abrCfg.MaxWidth > wssSoftMaxW {
			abrCfg.MaxWidth = wssSoftMaxW
		}
		if abrCfg.MaxHeight > wssSoftMaxH {
			abrCfg.MaxHeight = wssSoftMaxH
		}
		maxW, maxH = abrCfg.MaxWidth, abrCfg.MaxHeight
	}
	abr.ApplyOpenTarget(maxW, maxH, fps, bitrate, abrCfg)
	// Peek target; Snapshot clears wantKey — re-arm below.
	tw, th, tfps, tbr, _ := abr.Snapshot()
	abr.RequestKeyframe()
	wantHEVC := false
	if namer, ok := enc.(desktop.CodecNamer); ok {
		c := strings.ToLower(namer.CodecName())
		wantHEVC = strings.Contains(c, "265") || strings.Contains(c, "hevc")
	}
	// Queue encoder recreate on the pump goroutine. Never Close VT here —
	// mid-Encode destroy hung desktopPump (READY 672, pic stuck 1056).
	a.mu.Lock()
	needResize := a.deskEncW != tw || a.deskEncH != th
	if needResize {
		a.deskResizePending = true
		a.deskResizeW, a.deskResizeH = tw, th
		a.deskResizeFPS, a.deskResizeBR = tfps, tbr
		a.deskResizeHEVC = wantHEVC
	} else if rc, ok := enc.(desktop.Reconfigurer); ok {
		_ = rc.Reconfigure(tw, th, tfps, tbr)
	}
	a.mu.Unlock()
	a.deskForceKey.Store(true)
	if rk, ok := cap.(interface{ RequestKeyframe() }); ok {
		rk.RequestKeyframe()
	}
	abr.RequestKeyframe()
	codecName := desktop.CodecH264
	if namer, ok := enc.(desktop.CodecNamer); ok {
		codecName = namer.CodecName()
	}
	mons, _ := desktop.ListMonitors()
	dinfos := make([]re2.DisplayInfo, 0, len(mons))
	for _, m := range mons {
		di := re2.DisplayInfo{ID: m.ID, Name: m.Name, Width: m.Width, Height: m.Height, X: m.X, Y: m.Y, Primary: m.Primary}
		if desktop.IsVirtualDisplay(m.ID) {
			di.Virtual = true
			if fw, fh, ok := desktop.VirtualFramebuffer(m.ID); ok {
				di.FBWidth, di.FBHeight = fw, fh
			}
		}
		dinfos = append(dinfos, di)
	}
	vp := 0
	if wantVideoPlane {
		vp = 1
	}
	ready := re2.DesktopReadyPayload{
		SessionID:   data.SessionID,
		Width:       tw,
		Height:      th,
		Codec:       codecName,
		FPS:         tfps,
		BitrateKbps: tbr,
		DisplayID:   data.DisplayID,
		Displays:    dinfos,
		VideoPlane:  vp,
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		p := desktop.CheckHostPermissions()
		mic, cam := p.Microphone, p.Camera
		ready.MicAuthorized = &mic
		ready.CameraAuthorized = &cam
	}
	body := re2.MustJSON(ready)
	for len(dinfos) > 0 && a.useUDP && len(body)+16 > reudp.MaxPayload {
		dinfos = dinfos[:len(dinfos)-1]
		ready.Displays = dinfos
		body = re2.MustJSON(ready)
	}
	log.Printf("desktop soft reopen session=%s %dx%d@%d %dkbps plane=%d lan=%v resize=%v",
		data.SessionID, tw, th, tfps, tbr, vp, lanDirect, needResize)
	a.touchActivity()
	// Defer READY to the pump: it sends READY first, then a forced IDR (client
	// resets VT before the new SPS). Immediate READY raced in-flight old frames.
	a.mu.Lock()
	a.deskReadyPending = body
	a.deskForceKeyUntil = time.Now().Add(5 * time.Second)
	a.mu.Unlock()
	a.deskForceKey.Store(true)
	return nil
}

func (a *Agent) closeDesktop() {
	a.mu.Lock()
	cancel := a.deskCancel
	cap := a.deskCap
	inj := a.deskInj
	enc := a.deskEnc
	player := a.audioPlayer
	wasActive := a.deskSID != ""
	notify := a.onDesktopChange
	a.deskCancel = nil
	a.deskCap = nil
	a.deskInj = nil
	a.deskEnc = nil
	a.deskEncW = 0
	a.deskEncH = 0
	a.deskSID = ""
	a.deskABR = nil
	a.deskCapMaxW = 0
	a.deskCapMaxH = 0
	a.deskResizePending = false
	a.deskReadyPending = nil
	a.deskVideoPlane = false
	a.deskKeyFrames = nil
	a.deskKeyOrder = nil
	a.inputCh = nil
	a.inputNextID = 0
	a.inputPending = nil
	a.audioPlayer = nil
	if len(a.sessions) == 0 {
		a.controlPeer = ""
		a.controlSince = time.Time{}
	}
	a.controlDeskOn = false
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cap != nil {
		_ = cap.Close()
	}
	if inj != nil {
		_ = inj.Close()
	}
	if enc != nil {
		_ = enc.Close()
	}
	if player != nil {
		_ = player.Close()
	}
	a.closeCamera()
	a.closePhoneCam()
	_ = desktop.SetPrivacyBlank(false)
	if wasActive {
		audit.Log("desktop_close", "")
		if notify != nil {
			notify(false)
		}
	}
}

// resendVideoParts repairs sparse loss without forcing another full IDR. It is
// intentionally limited to the latest keyframe and the UDP video plane.
func (a *Agent) resendVideoParts(req re2.VideoNACKPayload) {
	a.mu.Lock()
	if !a.deskVideoPlane || req.SessionID == "" || req.SessionID != a.deskSID ||
		req.Missing == nil {
		a.mu.Unlock()
		return
	}
	cached, ok := a.deskKeyFrames[req.FrameID]
	if !ok || time.Since(cached.at) > 15*time.Second {
		a.mu.Unlock()
		return
	}
	parts := cached.parts
	vm := a.videoMedia
	ep := a.udpEP
	a.mu.Unlock()
	if vm == nil || ep == nil || len(parts) == 0 {
		return
	}
	missing := req.Missing
	if len(missing) > 256 {
		missing = missing[:256]
	}
	sent := 0
	seen := make(map[uint16]struct{}, len(missing))
	for _, index := range missing {
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		i := int(index)
		if i < 0 || i >= len(parts) {
			continue
		}
		inner := re2.EncodeInner(re2.MsgVideo, parts[i])
		ct, err := vm.Seal(inner)
		if err != nil || len(ct) > reudp.MaxPayload {
			continue
		}
		if err := ep.SendUnreliable(ct); err != nil {
			continue
		}
		sent++
		time.Sleep(200 * time.Microsecond)
	}
	if sent > 0 {
		log.Printf("video NACK repair frame=%d sent=%d/%d", req.FrameID, sent, len(missing))
	}
}

func (a *Agent) handleDesktopInput(mt byte, body []byte) error {
	a.mu.Lock()
	inj := a.deskInj
	a.mu.Unlock()
	if inj == nil {
		return nil
	}
	switch mt {
	case re2.MsgInputMouse:
		var p re2.InputMousePayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		if p.Gesture == "space" && p.SpaceDelta != 0 {
			delta := p.SpaceDelta
			injRef := inj
			// Never block the input pump on Dock swipe usleep (~150ms+) — under WSS
			// that stalls clicks/keys and a rapid second swipe collides mid-animation.
			go func() {
				if err := desktop.NudgeDesktopSpace(delta); err != nil {
					log.Printf("input gesture=space delta=%d err=%v — fallback Control+Arrow", delta, err)
					code := 123
					if delta > 0 {
						code = 124
					}
					_ = injRef.Key(code, "", true, 2)
					_ = injRef.Key(code, "", false, 2)
				} else {
					log.Printf("input gesture=space delta=%d ok", delta)
				}
			}()
			return nil
		}
		// Sparse input trace for gesture self-test (click / scroll / drag).
		if p.Down || p.Up || p.Wheel != 0 || p.WheelH != 0 {
			log.Printf("input mouse down=%v up=%v btn=%d wheel=%d wheelH=%d rel=%v",
				p.Down, p.Up, p.Buttons, p.Wheel, p.WheelH, p.Relative)
		}
		a.mu.Lock()
		relMode := a.relativeMouse
		a.mu.Unlock()
		if p.Relative || (relMode && (p.DX != 0 || p.DY != 0)) {
			_ = inj.MoveRelative(p.DX, p.DY)
		} else if p.Move || (!p.Down && !p.Up) {
			_ = inj.Move(p.X, p.Y)
		}
		if p.Down {
			if !p.Relative {
				_ = inj.Move(p.X, p.Y)
			}
			_ = inj.Button(p.Buttons, true)
		}
		if p.Up {
			_ = inj.Button(p.Buttons, false)
		}
		if p.Wheel != 0 {
			_ = inj.Wheel(p.Wheel)
		}
		if p.WheelH != 0 {
			_ = inj.WheelH(p.WheelH)
		}
	case re2.MsgInputKey:
		var p re2.InputKeyPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		return inj.Key(p.KeyCode, p.Text, p.Down, p.Modifiers)
	case re2.MsgInputTouch:
		var p re2.InputTouchPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		_ = inj.Move(p.X, p.Y)
		switch p.Phase {
		case "begin":
			_ = inj.Button(1, true)
		case "end":
			_ = inj.Button(1, false)
		}
	}
	return nil
}

func (a *Agent) handleHolePunch(hp re2.HolePunchPayload) error {
	switch hp.Action {
	case "offer", "candidate":
		cands := a.localUDPCandidates()
		_ = a.sendTunnel(re2.MsgHolePunch, re2.MustJSON(re2.HolePunchPayload{
			Action: "candidate", Token: hp.Token, Candidates: cands, UDPAddr: firstNonEmpty(cands, hp.UDPAddr),
		}), true)
		targets := append([]string{}, hp.Candidates...)
		if hp.UDPAddr != "" {
			targets = append(targets, hp.UDPAddr)
		}
		for _, addr := range targets {
			if addr == "" || strings.HasSuffix(addr, ":0") {
				continue
			}
			go func(peer string) {
				a.mu.Lock()
				ep := a.udpEP
				a.mu.Unlock()
				if ep == nil {
					return
				}
				// Prefer main-listen REHP1 (same port as QR lan) over ephemeral DialUDP.
				if err := ep.ProbePreferDirect(peer, hp.Token, 3*time.Second); err != nil {
					conn, err2 := holepunch.TryDirect(0, peer, hp.Token, 3*time.Second)
					if err2 != nil {
						return
					}
					ep.PreferDirect(conn)
				}
				audit.Log("holepunch_direct", peer)
				_ = a.sendTunnel(re2.MsgHolePunch, re2.MustJSON(re2.HolePunchPayload{
					Action: "connected", Token: hp.Token, UDPAddr: peer,
				}), true)
			}(addr)
		}
	case "connected":
		// Peer already prefers direct; if we have their addr and no direct yet, try once.
		if hp.UDPAddr == "" || strings.HasSuffix(hp.UDPAddr, ":0") {
			return nil
		}
		a.mu.Lock()
		ep := a.udpEP
		already := ep != nil && ep.UsingDirect()
		a.mu.Unlock()
		if already || ep == nil {
			return nil
		}
		go func(peer string) {
			if err := ep.ProbePreferDirect(peer, hp.Token, 2*time.Second); err != nil {
				conn, err2 := holepunch.TryDirect(0, peer, hp.Token, 2*time.Second)
				if err2 != nil {
					return
				}
				ep.PreferDirect(conn)
			}
			audit.Log("holepunch_direct_ack", peer)
		}(hp.UDPAddr)
	}
	return nil
}

func firstNonEmpty(cands []string, fallback string) string {
	for _, c := range cands {
		if c != "" && !strings.HasSuffix(c, ":0") {
			return c
		}
	}
	return fallback
}

func (a *Agent) handleFileMsg(mt byte, body []byte) error {
	dir, err := desktop.XferDir()
	if err != nil {
		return err
	}
	switch mt {
	case re2.MsgFileOffer:
		var of re2.FileOfferPayload
		if err := json.Unmarshal(body, &of); err != nil {
			return err
		}
		name := sanitizeXferName(of.Name)
		if name == "" {
			name = sanitizeXferName(of.FileID)
		}
		if name == "" {
			name = "upload.bin"
		}
		a.mu.Lock()
		if a.xferNames == nil {
			a.xferNames = make(map[string]string)
		}
		a.xferNames[of.FileID] = name
		a.mu.Unlock()
		path := filepath.Join(dir, name)
		_ = os.Remove(path) // fresh transfer
		audit.Log("file_offer", name)
		log.Printf("file RX OFFER id=%s name=%s size=%d → ACK", of.FileID, name, of.Size)
		if err := a.sendTunnel(re2.MsgFileAck, re2.MustJSON(re2.FileAckPayload{SessionID: of.SessionID, FileID: of.FileID, OK: true}), true); err != nil {
			log.Printf("file TX ACK err=%v", err)
			return err
		}
		return nil
	case re2.MsgFileChunk:
		var ch re2.FileChunkPayload
		if err := json.Unmarshal(body, &ch); err != nil {
			return err
		}
		raw, err := base64.StdEncoding.DecodeString(ch.DataB64)
		if err != nil {
			return err
		}
		a.mu.Lock()
		name := a.xferNames[ch.FileID]
		a.mu.Unlock()
		if name == "" {
			name = sanitizeXferName(ch.FileID)
			if name == "" {
				name = "upload.bin"
			}
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, err = f.WriteAt(raw, ch.Offset)
		_ = f.Close()
		if err != nil {
			return err
		}
		if ch.EOF {
			audit.Log("file_done", name)
		}
		return a.sendTunnel(re2.MsgFileAck, re2.MustJSON(re2.FileAckPayload{
			SessionID: ch.SessionID, FileID: ch.FileID, Offset: ch.Offset + int64(len(raw)), OK: true,
		}), true)
	case re2.MsgFilePull:
		var pull re2.FilePullPayload
		if err := json.Unmarshal(body, &pull); err != nil {
			return err
		}
		log.Printf("file RX PULL path=%q id=%s", pull.Path, pull.FileID)
		go a.pushFileToClient(pull, dir)
		return nil
	}
	return nil
}

func (a *Agent) pushFileToClient(pull re2.FilePullPayload, xferDir string) {
	path, err := resolveXferPath(pull.Path, xferDir)
	if err != nil {
		_ = a.sendTunnel(re2.MsgFileAck, re2.MustJSON(re2.FileAckPayload{
			SessionID: pull.SessionID, FileID: pull.FileID, OK: false, Error: err.Error(),
		}), true)
		return
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		_ = a.sendTunnel(re2.MsgFileAck, re2.MustJSON(re2.FileAckPayload{
			SessionID: pull.SessionID, FileID: pull.FileID, OK: false, Error: "not found",
		}), true)
		return
	}
	name := filepath.Base(path)
	fileID := pull.FileID
	if fileID == "" {
		fileID = name
	}
	_ = a.sendTunnel(re2.MsgFileOffer, re2.MustJSON(re2.FileOfferPayload{
		SessionID: pull.SessionID, FileID: fileID, Name: name, Size: st.Size(),
	}), true)
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	off := pull.ResumeFrom
	if off > 0 {
		if _, err := f.Seek(off, io.SeekStart); err != nil {
			off = 0
			_, _ = f.Seek(0, io.SeekStart)
		}
	}
	// REUDP MaxPayload≈1200; 48KiB chunks never fit Noise ciphertext on PreferDirect.
	chunkSize := 48 * 1024
	a.mu.Lock()
	udp := a.useUDP
	a.mu.Unlock()
	if udp {
		chunkSize = 700
	}
	buf := make([]byte, chunkSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			eof := err == io.EOF || off+int64(n) >= st.Size()
			prog := float64(off+int64(n)) / float64(st.Size())
			body := re2.MustJSON(re2.FileChunkPayload{
				SessionID: pull.SessionID, FileID: fileID, Offset: off,
				DataB64: base64.StdEncoding.EncodeToString(buf[:n]), EOF: eof,
			})
			if err := a.sendTunnel(re2.MsgFileChunk, body, true); err != nil {
				log.Printf("file TX CHUNK err=%v off=%d n=%d", err, off, n)
				return
			}
			_ = a.sendTunnel(re2.MsgFileAck, re2.MustJSON(re2.FileAckPayload{
				SessionID: pull.SessionID, FileID: fileID, Offset: off + int64(n), OK: true, Progress: prog,
			}), true)
			off += int64(n)
		}
		if err == io.EOF || off >= st.Size() {
			audit.Log("file_push_done", name)
			log.Printf("file TX PULL done name=%s bytes=%d", name, off)
			return
		}
		if err != nil {
			log.Printf("file TX PULL read err=%v", err)
			return
		}
	}
}

func resolveXferPath(req, xferDir string) (string, error) {
	req = strings.TrimSpace(req)
	if req == "" {
		return "", errString("empty path")
	}
	root := strings.TrimSpace(os.Getenv("RE_XFER_ROOT"))
	if root == "" {
		root = xferDir
	}
	root, _ = filepath.Abs(root)
	var abs string
	if filepath.IsAbs(req) {
		abs, _ = filepath.Abs(req)
	} else {
		abs, _ = filepath.Abs(filepath.Join(xferDir, filepath.Base(req)))
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		// also allow xferDir itself
		xferAbs, _ := filepath.Abs(xferDir)
		rel2, err2 := filepath.Rel(xferAbs, abs)
		if err2 != nil || strings.HasPrefix(rel2, "..") {
			return "", errString("path not allowed")
		}
	}
	return abs, nil
}

func sanitizeXferName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "..", "_")
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	return name
}
