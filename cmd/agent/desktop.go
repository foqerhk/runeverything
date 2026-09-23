package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/audit"
	"github.com/foqerhk/runeverything/internal/deskbridge"
	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/holepunch"
	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/reudp"
)

func (a *Agent) touchActivity() {
	a.mu.Lock()
	a.lastActivity = time.Now()
	a.mu.Unlock()
}

func (a *Agent) startUDP(udpHostPort string) error {
	if udpHostPort == "" {
		return nil
	}
	ep, err := reudp.Dial(udpHostPort)
	if err != nil {
		return err
	}
	if err := ep.AssocAgent(a.id.DeviceID, a.id.DeviceSecret); err != nil {
		_ = ep.Close()
		return err
	}
	ok, err := ep.WaitAssocOK(10 * time.Second)
	if err != nil {
		_ = ep.Close()
		return err
	}
	i18n.Log("log.reudp_associated", ok.DeviceID, ok.UDPHint)
	a.mu.Lock()
	if a.udpEP != nil {
		_ = a.udpEP.Close()
	}
	a.udpEP = ep
	a.mu.Unlock()
	go a.udpReadLoop()
	return nil
}

func (a *Agent) udpReadLoop() {
	for {
		a.mu.Lock()
		ep := a.udpEP
		idle := a.sessionIdle
		last := a.lastActivity
		a.mu.Unlock()
		if ep == nil {
			return
		}
		if idle > 0 && !last.IsZero() && time.Since(last) > idle {
			i18n.Log("log.session_idle", idle)
			audit.Log("session_idle_timeout", a.id.DeviceID)
			a.closeDesktop()
			a.re2Sess = nil
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
	if a.re2Sess == nil {
		if a.pairingToken == "" {
			return nil
		}
		psk := re2.DerivePSK(a.pairingToken)
		hs, err := re2.NewAgentHandshake(psk, a.noiseKP)
		if err != nil {
			return err
		}
		tr := &udpPendingTransport{first: payload, ep: a.udpEP}
		sess, _, err := hs.RunAgent(tr)
		if err != nil {
			return err
		}
		a.re2Sess = sess
		a.useUDP = true
		a.touchActivity()
		audit.Log("noise_ok", "udp")
		i18n.Log("log.re2_noise_udp", a.id.DeviceID)
		return nil
	}
	a.touchActivity()
	plain, err := a.re2Sess.Decrypt(payload)
	if err != nil {
		return err
	}
	return a.handleRE2Inner(plain)
}

type udpPendingTransport struct {
	first []byte
	ep    *reudp.Endpoint
}

func (t *udpPendingTransport) Send(msg []byte) error {
	return t.ep.SendReliable(msg)
}

func (t *udpPendingTransport) Recv() ([]byte, error) {
	if t.first != nil {
		m := t.first
		t.first = nil
		return m, nil
	}
	return t.ep.Recv()
}

func (a *Agent) sendTunnel(msgType byte, body []byte, reliable bool) error {
	if a.re2Sess == nil {
		return errNoSession
	}
	ct, err := a.re2Sess.Encrypt(re2.EncodeInner(msgType, body))
	if err != nil {
		return err
	}
	a.mu.Lock()
	ep := a.udpEP
	useUDP := a.useUDP && ep != nil
	conn := a.re2Conn
	a.mu.Unlock()
	if useUDP {
		if reliable {
			return ep.SendReliable(ct)
		}
		return ep.SendUnreliable(ct)
	}
	if conn == nil {
		return errNoSession
	}
	return conn.WriteFrame(re2.Frame{Type: re2.TypeTunnel, RouteID: a.id.DeviceID, Payload: ct})
}

var errNoSession = errString("session not ready")

type errString string

func (e errString) Error() string { return string(e) }

func (a *Agent) openDesktop(data re2.OpenDesktopPayload) error {
	if err := a.checkDesktopAccess(data.Password); err != nil {
		audit.Log("desktop_denied", err.Error())
		return err
	}
	a.closeDesktop()
	desktop.SetSelectedMonitor(data.DisplayID)
	desktop.SetHideCursor(data.HideCursor)
	if data.PrivacyBlank {
		_ = desktop.SetPrivacyBlank(true)
	} else {
		_ = desktop.SetPrivacyBlank(false)
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

	cap, err := deskbridge.NewCapturer()
	if err != nil {
		return err
	}
	inj, err := deskbridge.NewInjector()
	if err != nil {
		_ = cap.Close()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	frames, err := cap.Start(ctx, maxW, maxH)
	if err != nil {
		cancel()
		_ = cap.Close()
		_ = inj.Close()
		return err
	}
	var first desktop.Frame
	select {
	case first = <-frames:
	case <-time.After(5 * time.Second):
		cancel()
		_ = cap.Close()
		_ = inj.Close()
		return errString("desktop capture timeout")
	}
	w, h := first.Img.Bounds().Dx(), first.Img.Bounds().Dy()
	enc, err := desktop.NewEncoderBitrate(w, h, fps, bitrate)
	if err != nil {
		cancel()
		_ = cap.Close()
		_ = inj.Close()
		return err
	}
	if di, ok := inj.(interface{ SetScreenSize(int, int) }); ok {
		di.SetScreenSize(w, h)
	}
	abr := desktop.NewABR(desktop.DefaultABR(), w, h, fps, bitrate)
	mons, _ := desktop.ListMonitors()
	dinfos := make([]re2.DisplayInfo, 0, len(mons))
	for _, m := range mons {
		dinfos = append(dinfos, re2.DisplayInfo{ID: m.ID, Name: m.Name, Width: m.Width, Height: m.Height, X: m.X, Y: m.Y, Primary: m.Primary})
	}

	a.mu.Lock()
	a.deskCancel = cancel
	a.deskCap = cap
	a.deskInj = inj
	a.deskEnc = enc
	a.deskSID = data.SessionID
	a.deskABR = abr
	a.mu.Unlock()
	a.touchActivity()
	audit.Log("desktop_open", data.SessionID)

	go a.desktopPump(frames, first, data.SessionID, fps)
	go a.cursorPump(data.SessionID, ctx)
	go a.audioPump(data.SessionID, ctx)
	go a.clipboardPump(data.SessionID, ctx)
	return a.sendTunnel(re2.MsgDesktopReady, re2.MustJSON(re2.DesktopReadyPayload{
		SessionID:   data.SessionID,
		Width:       w,
		Height:      h,
		Codec:       desktop.CodecH264,
		FPS:         fps,
		BitrateKbps: bitrate,
		DisplayID:   data.DisplayID,
		Displays:    dinfos,
	}), true)
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
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pos, err := cr.Cursor()
			if err != nil {
				continue
			}
			_ = a.sendTunnel(re2.MsgCursor, re2.MustJSON(re2.CursorPayload{
				SessionID: sid, X: pos.X, Y: pos.Y, Visible: pos.Visible,
			}), false)
		}
	}
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
	sendFrame := func(f desktop.Frame, key bool) {
		a.mu.Lock()
		enc := a.deskEnc
		sess := a.re2Sess
		ep := a.udpEP
		useUDP := a.useUDP
		conn := a.re2Conn
		abr := a.deskABR
		fid := a.deskFrameID
		a.deskFrameID++
		a.mu.Unlock()
		if enc == nil || sess == nil || f.Img == nil {
			return
		}
		forceKey := key
		if abr != nil {
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
			if tw > 0 && th > 0 && (srcW != tw || srcH != th) {
				f.Img = desktop.ScaleExact(f.Img, tw, th)
			}
			if rc, ok := enc.(desktop.Reconfigurer); ok {
				_ = rc.Reconfigure(tw, th, tfps, tbr)
			}
		}
		annexB, err := enc.Encode(f, forceKey)
		if err != nil || len(annexB) == 0 {
			return
		}
		flags := byte(0)
		if forceKey {
			flags = re2.VideoFlagKeyFrame
		}
		parts := re2.FragmentNAL(sid, fid, flags, annexB, reudp.MaxPayload-64)
		for _, part := range parts {
			ct, err := sess.Encrypt(re2.EncodeInner(re2.MsgVideo, part))
			if err != nil {
				return
			}
			if ep != nil && useUDP {
				_ = ep.SendUnreliable(ct)
			} else if conn != nil {
				_ = conn.WriteFrame(re2.Frame{Type: re2.TypeTunnel, RouteID: a.id.DeviceID, Payload: ct})
			}
		}
	}
	sendFrame(first, true)
	n := 0
	for f := range frames {
		n++
		sendFrame(f, n%30 == 0)
		a.mu.Lock()
		alive := a.deskSID == sid
		a.mu.Unlock()
		if !alive {
			return
		}
	}
}

func (a *Agent) closeDesktop() {
	a.mu.Lock()
	cancel := a.deskCancel
	cap := a.deskCap
	inj := a.deskInj
	enc := a.deskEnc
	player := a.audioPlayer
	a.deskCancel = nil
	a.deskCap = nil
	a.deskInj = nil
	a.deskEnc = nil
	a.deskSID = ""
	a.deskABR = nil
	a.audioPlayer = nil
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
	_ = desktop.SetPrivacyBlank(false)
	audit.Log("desktop_close", "")
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
		cands := localUDPCandidates()
		_ = a.sendTunnel(re2.MsgHolePunch, re2.MustJSON(re2.HolePunchPayload{
			Action: "candidate", Token: hp.Token, Candidates: cands, UDPAddr: hp.UDPAddr,
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
				conn, err := holepunch.TryDirect(0, peer, hp.Token, 2*time.Second)
				if err != nil {
					return
				}
				_ = conn.Close()
				_ = a.sendTunnel(re2.MsgHolePunch, re2.MustJSON(re2.HolePunchPayload{Action: "connected", Token: hp.Token, UDPAddr: peer}), true)
				audit.Log("holepunch_ok", peer)
			}(addr)
		}
	}
	return nil
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
		return a.sendTunnel(re2.MsgFileAck, re2.MustJSON(re2.FileAckPayload{SessionID: of.SessionID, FileID: of.FileID, OK: true}), true)
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
	const chunkSize = 48 * 1024
	buf := make([]byte, chunkSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			eof := err == io.EOF || off+int64(n) >= st.Size()
			prog := float64(off+int64(n)) / float64(st.Size())
			_ = a.sendTunnel(re2.MsgFileChunk, re2.MustJSON(re2.FileChunkPayload{
				SessionID: pull.SessionID, FileID: fileID, Offset: off,
				DataB64: base64.StdEncoding.EncodeToString(buf[:n]), EOF: eof,
			}), true)
			_ = a.sendTunnel(re2.MsgFileAck, re2.MustJSON(re2.FileAckPayload{
				SessionID: pull.SessionID, FileID: fileID, Offset: off + int64(n), OK: true, Progress: prog,
			}), false)
			off += int64(n)
		}
		if err == io.EOF || off >= st.Size() {
			audit.Log("file_push_done", name)
			return
		}
		if err != nil {
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
