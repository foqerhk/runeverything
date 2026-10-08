package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/audit"
	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/netutil"
	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/reudp"
)

func (a *Agent) handlePeripheral(mt byte, body []byte) error {
	switch mt {
	case re2.MsgWakeOnLAN:
		var p re2.WakeOnLANPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		if err := desktop.WakeOnLAN(p.MAC, p.Broadcast); err != nil {
			return a.sendRE2AppErr("wol_failed", err.Error())
		}
		audit.Log("wol", p.MAC)
		return nil

	case re2.MsgCameraList:
		devs, _ := desktop.ListCameras()
		out := make([]re2.CameraDevice, 0, len(devs))
		for _, d := range devs {
			out = append(out, re2.CameraDevice{ID: d.ID, Name: d.Name})
		}
		return a.sendTunnel(re2.MsgCameraList, re2.MustJSON(re2.CameraListPayload{Devices: out}), true)

	case re2.MsgCameraOpen:
		var p re2.CameraOpenPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		a.closeCamera()
		ctx, cancel := context.WithCancel(context.Background())
		cam, err := desktop.StartCamera(ctx, p.DeviceID, p.Width, p.Height, p.FPS, p.Codec)
		if err != nil {
			cancel()
			return a.sendRE2AppErr("camera_open_failed", err.Error())
		}
		a.mu.Lock()
		a.camCancel = cancel
		a.cam = cam
		a.mu.Unlock()
		go a.cameraPump(p.SessionID, cam, ctx)
		audit.Log("camera_open", p.DeviceID)
		return nil

	case re2.MsgCameraClose:
		a.closeCamera()
		return nil

	case re2.MsgPhoneCamOpen:
		var p re2.PhoneCamOpenPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		a.closePhoneCam()
		codec := strings.ToLower(strings.TrimSpace(p.Codec))
		if codec == "" {
			codec = "mjpeg"
		}
		log.Printf("phonecam OPEN codec=%s %dx%d@%d agent=%s", codec, p.Width, p.Height, p.FPS, version)
		sink, err := desktop.StartPhoneCamSink(p.Width, p.Height, p.FPS)
		if err != nil {
			_ = a.sendTunnel(re2.MsgPhoneCamReady, re2.MustJSON(re2.PhoneCamReadyPayload{
				SessionID: p.SessionID, OK: false, Error: err.Error(),
			}), true)
			return a.sendRE2AppErr("phonecam_open_failed", err.Error())
		}
		a.mu.Lock()
		a.phoneCam = sink
		a.phoneCamSID = p.SessionID
		a.mu.Unlock()
		audit.Log("phonecam_open", sink.DeviceName())
		return a.sendTunnel(re2.MsgPhoneCamReady, re2.MustJSON(re2.PhoneCamReadyPayload{
			SessionID: p.SessionID, OK: true, Device: sink.DeviceName(),
		}), true)

	case re2.MsgPhoneCamClose:
		a.closePhoneCam()
		return nil

	case re2.MsgPhoneCamFrame:
		return a.handlePhoneCamFrame(body)

	case re2.MsgUSBList:
		devs, err := desktop.ListUSBDevices()
		out := re2.USBListPayload{}
		if err != nil {
			out.Error = err.Error()
		}
		for _, d := range devs {
			out.Devices = append(out.Devices, re2.USBDevice{
				ID: d.ID, Name: d.Name, VendorID: d.VendorID, ProductID: d.ProductID, Bus: d.Bus,
			})
		}
		return a.sendTunnel(re2.MsgUSBList, re2.MustJSON(out), true)

	case re2.MsgUSBAttach:
		var p re2.USBAttachPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		if err := desktop.USBAttach(p.DeviceID, p.RemoteAddr); err != nil {
			return a.sendRE2AppErr("usb_attach_failed", err.Error())
		}
		audit.Log("usb_attach", p.DeviceID)
		return nil

	case re2.MsgUSBDetach:
		var p re2.USBDetachPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		_ = desktop.USBDetach(p.DeviceID)
		audit.Log("usb_detach", p.DeviceID)
		return nil

	case re2.MsgPrinterList:
		ps, err := desktop.ListPrinters()
		out := re2.PrinterListPayload{}
		if err == nil {
			for _, p := range ps {
				out.Printers = append(out.Printers, re2.PrinterInfo{ID: p.ID, Name: p.Name, Default: p.Default})
			}
		}
		return a.sendTunnel(re2.MsgPrinterList, re2.MustJSON(out), true)

	case re2.MsgPrinterJob:
		var p re2.PrinterJobPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		raw, err := desktop.DecodeB64(p.DataB64)
		if err != nil {
			return a.sendTunnel(re2.MsgPrinterAck, re2.MustJSON(re2.PrinterAckPayload{
				SessionID: p.SessionID, JobID: p.JobID, OK: false, Error: err.Error(),
			}), true)
		}
		err = desktop.SubmitPrintJob(p.PrinterID, p.Name, p.Mime, raw)
		ack := re2.PrinterAckPayload{SessionID: p.SessionID, JobID: p.JobID, OK: err == nil}
		if err != nil {
			ack.Error = err.Error()
		} else {
			audit.Log("print_job", p.Name)
		}
		return a.sendTunnel(re2.MsgPrinterAck, re2.MustJSON(ack), true)

	case re2.MsgFileList:
		var p re2.FileListPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		log.Printf("file RX LIST path=%q sid=%s", p.Path, p.SessionID)
		return a.handleFileList(p)

	case re2.MsgInputMode:
		var p re2.InputModePayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		a.mu.Lock()
		inj := a.deskInj
		a.relativeMouse = p.RelativeMouse || p.GameMode
		a.mu.Unlock()
		if inj != nil {
			inj.SetRelativeMouse(p.RelativeMouse || p.GameMode)
		}
		return nil
	}
	return nil
}

func (a *Agent) handleFileList(p re2.FileListPayload) error {
	dir, err := desktop.XferDir()
	if err != nil {
		log.Printf("file LIST xferdir err=%v", err)
		return a.sendTunnel(re2.MsgFileList, re2.MustJSON(re2.FileListPayload{SessionID: p.SessionID, Error: err.Error()}), true)
	}
	root := strings.TrimSpace(os.Getenv("RE_XFER_ROOT"))
	if root == "" {
		root = dir
	}
	root, _ = filepath.Abs(root)
	req := strings.TrimSpace(p.Path)
	target := root
	if req != "" {
		if filepath.IsAbs(req) {
			target, _ = filepath.Abs(req)
		} else {
			target, _ = filepath.Abs(filepath.Join(root, req))
		}
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		log.Printf("file LIST deny root=%s target=%s rel=%q", root, target, rel)
		return a.sendTunnel(re2.MsgFileList, re2.MustJSON(re2.FileListPayload{SessionID: p.SessionID, Error: "path not allowed"}), true)
	}
	entries, err := os.ReadDir(target)
	out := re2.FileListPayload{SessionID: p.SessionID, Path: rel}
	if err != nil {
		out.Error = err.Error()
		log.Printf("file LIST readdir err=%v", err)
		return a.sendTunnel(re2.MsgFileList, re2.MustJSON(out), true)
	}
	for _, e := range entries {
		info, _ := e.Info()
		ent := re2.FileListEntry{Name: e.Name(), IsDir: e.IsDir()}
		if info != nil {
			ent.Size = info.Size()
			ent.ModUnix = info.ModTime().Unix()
		}
		out.Entries = append(out.Entries, ent)
	}
	body := re2.MustJSON(out)
	// PreferDirect: keep the LIST reply under REUDP MaxPayload or the client
	// never sees entries (sendTunnel returns ErrTooLarge before encrypt).
	const noiseTag = 16
	for a.useUDP && len(re2.EncodeInner(re2.MsgFileList, body))+noiseTag > reudp.MaxPayload && len(out.Entries) > 1 {
		out.Entries = out.Entries[:len(out.Entries)/2]
		body = re2.MustJSON(out)
	}
	if a.useUDP && len(re2.EncodeInner(re2.MsgFileList, body))+noiseTag > reudp.MaxPayload {
		// Still too big with one entry — return error instead of silent drop.
		out.Entries = nil
		out.Error = "listing too large for UDP"
		body = re2.MustJSON(out)
	}
	log.Printf("file TX LIST path=%q entries=%d bytes=%d", rel, len(out.Entries), len(body))
	if err := a.sendTunnel(re2.MsgFileList, body, true); err != nil {
		log.Printf("file TX LIST send err=%v", err)
		return err
	}
	return nil
}

func (a *Agent) cameraPump(sid string, cam *desktop.CameraCapture, ctx context.Context) {
	defer a.closeCamera()
	var frameID uint32
	// Keep each JSON body small enough that Noise ciphertext fits REUDP MaxPayload (1200).
	const chunkRaw = 700
	// Cap parts so one preview frame can't flood the reliable queue (~12KB JPEG).
	const maxParts = 18
	for {
		select {
		case <-ctx.Done():
			return
		default:
			frame, err := cam.ReadFrame()
			if err != nil || len(frame) == 0 {
				time.Sleep(40 * time.Millisecond)
				continue
			}
			if len(frame) > chunkRaw*maxParts {
				log.Printf("camera skip oversized frame bytes=%d (max=%d)", len(frame), chunkRaw*maxParts)
				time.Sleep(80 * time.Millisecond)
				continue
			}
			w, h := cam.Size()
			frameID++
			parts := uint16((len(frame) + chunkRaw - 1) / chunkRaw)
			if parts == 0 {
				parts = 1
			}
			key := cam.Codec() != "h264"
			sendFailed := false
			for p := uint16(0); p < parts; p++ {
				start := int(p) * chunkRaw
				end := start + chunkRaw
				if end > len(frame) {
					end = len(frame)
				}
				body := re2.MustJSON(re2.CameraFramePayload{
					SessionID: sid, Codec: cam.Codec(), Width: w, Height: h,
					DataB64: desktop.EncodeB64(frame[start:end]), KeyFrame: key && p == 0,
					FrameID: frameID, Part: p, Parts: parts,
				})
				// Must be reliable: PreferDirect still uses REUDP MaxPayload; unreliable
				// drops + oversized ciphertext meant "camera on" never painted on the phone.
				if err := a.sendTunnel(re2.MsgCameraFrame, body, true); err != nil {
					log.Printf("camera frame send err frame=%d part=%d/%d: %v", frameID, p+1, parts, err)
					sendFailed = true
					// Abort remaining parts of this frame — partial assemblies never decode.
					if strings.Contains(err.Error(), "session not ready") {
						time.Sleep(250 * time.Millisecond)
					} else {
						time.Sleep(40 * time.Millisecond)
					}
					break
				}
			}
			if sendFailed {
				continue
			}
			// Pace preview (~8–10fps). Don't outrun the reliable tunnel.
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (a *Agent) closeCamera() {
	a.mu.Lock()
	cancel := a.camCancel
	cam := a.cam
	a.camCancel = nil
	a.cam = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cam != nil {
		_ = cam.Close()
	}
}

func (a *Agent) closePhoneCam() {
	a.mu.Lock()
	sink := a.phoneCam
	a.phoneCam = nil
	a.phoneCamSID = ""
	a.phoneCamFrameID = 0
	a.phoneCamParts = nil
	a.phoneCamExpected = 0
	a.phoneCamFrameKey = false
	a.phoneCamFramesOK.Store(0)
	a.mu.Unlock()
	if sink != nil {
		_ = sink.Close()
		audit.Log("phonecam_close", sink.DeviceName())
	}
}

func (a *Agent) handlePhoneCamFrame(body []byte) error {
	var (
		raw    []byte
		fid    uint32
		part   int
		parts  int
		w, h   int
		codec  string
		isKey  bool
		okSink bool
	)
	if re2.IsPhoneCamBinary(body) {
		sid, frameID, flags, pIdx, pTotal, width, height, cByte, chunk, err := re2.DecodePhoneCam(body)
		if err != nil || len(chunk) == 0 {
			return nil
		}
		_ = sid
		raw, fid, part, parts = chunk, frameID, int(pIdx), int(pTotal)
		w, h = width, height
		codec = re2.CodecString(cByte)
		isKey = flags&re2.PhoneCamFlagKey != 0
	} else {
		var p re2.CameraFramePayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		chunk, err := desktop.DecodeB64(p.DataB64)
		if err != nil || len(chunk) == 0 {
			return nil
		}
		raw, fid, part, parts = chunk, p.FrameID, int(p.Part), int(p.Parts)
		w, h = p.Width, p.Height
		codec = strings.ToLower(strings.TrimSpace(p.Codec))
		isKey = p.KeyFrame
	}
	if codec == "" {
		codec = "mjpeg"
	}
	if parts <= 0 {
		parts = 1
	}

	a.mu.Lock()
	sink := a.phoneCam
	if sink == nil {
		a.mu.Unlock()
		if fid == 1 && part == 0 {
			log.Printf("phonecam RX frame=1 but sink=nil (OPEN missed?) codec=%s parts=%d", codec, parts)
		}
		return nil
	}
	okSink = true
	var frame []byte
	if parts <= 1 {
		frame = raw
		if a.phoneCamFramesOK.Load() == 0 {
			log.Printf("phonecam assemble start frame=%d parts=1 codec=%s bytes=%d", fid, codec, len(raw))
		}
	} else {
		if a.phoneCamFrameID != fid {
			a.phoneCamFrameID = fid
			a.phoneCamParts = map[int][]byte{}
			a.phoneCamExpected = parts
			a.phoneCamFrameKey = isKey && part == 0
			n := a.phoneCamFramesOK.Load()
			if n == 0 || n%60 == 0 {
				log.Printf("phonecam assemble start frame=%d parts=%d codec=%s", fid, parts, codec)
			}
		}
		if part == 0 && isKey {
			a.phoneCamFrameKey = true
		}
		a.phoneCamParts[part] = raw
		if len(a.phoneCamParts) < a.phoneCamExpected {
			a.mu.Unlock()
			return nil
		}
		var assembled []byte
		for i := 0; i < a.phoneCamExpected; i++ {
			piece, ok := a.phoneCamParts[i]
			if !ok {
				a.mu.Unlock()
				return nil
			}
			assembled = append(assembled, piece...)
		}
		isKey = a.phoneCamFrameKey
		a.phoneCamParts = nil
		a.phoneCamFrameID = 0
		a.phoneCamExpected = 0
		a.phoneCamFrameKey = false
		frame = assembled
	}
	a.mu.Unlock()
	if !okSink || len(frame) == 0 {
		return nil
	}

	// Never block RE2/UDP on AkVCam stdin backpressure — that froze remote desktop.
	// H264 keys bypass the busy drop so the decoder can resync.
	busyOK := a.phoneCamWriteBusy.CompareAndSwap(false, true)
	if !busyOK {
		if codec == "h264" && isKey {
			deadline := time.Now().Add(40 * time.Millisecond)
			for !busyOK && time.Now().Before(deadline) {
				time.Sleep(2 * time.Millisecond)
				busyOK = a.phoneCamWriteBusy.CompareAndSwap(false, true)
			}
		}
		if !busyOK {
			return nil
		}
	}
	key := isKey
	go func() {
		defer a.phoneCamWriteBusy.Store(false)
		err := sink.WriteFrame(codec, frame, w, h, key)
		n := a.phoneCamFramesOK.Add(1)
		if err != nil {
			if n%30 == 1 {
				log.Printf("phonecam write err codec=%s bytes=%d: %v", codec, len(frame), err)
			}
			return
		}
		if n == 1 || n%60 == 0 {
			log.Printf("phonecam write ok n=%d codec=%s bytes=%d %dx%d key=%v", n, codec, len(frame), w, h, key)
		}
	}()
	return nil
}

func (a *Agent) localUDPCandidates() []string {
	var out []string
	port := 0
	a.mu.Lock()
	if a.udpEP != nil {
		if ua, ok := a.udpEP.LocalAddr().(*net.UDPAddr); ok && ua != nil {
			port = ua.Port
		}
	}
	a.mu.Unlock()
	if port <= 0 {
		return out
	}
	for _, ip := range netutil.LocalLANIPv4s() {
		out = append(out, net.JoinHostPort(ip.String(), itoaPort(port)))
	}
	return out
}

// lanPairingCandidates returns RFC1918 IPv4 host:port entries for the QR so a
// same-LAN scanner can prefer direct UDP without waiting on relay hole-punch.
// Never emits ip:0 — App drops those and Retry P2P becomes a no-op.
func (a *Agent) lanPairingCandidates() []string {
	port := a.waitUDPListenPort(3 * time.Second)
	if port <= 0 {
		return nil
	}
	ips := netutil.LocalLANIPv4s()
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, net.JoinHostPort(ip.String(), itoaPort(port)))
	}
	return out
}

func (a *Agent) waitUDPListenPort(budget time.Duration) int {
	deadline := time.Now().Add(budget)
	for {
		a.mu.Lock()
		port := 0
		if a.udpEP != nil {
			if ua, ok := a.udpEP.LocalAddr().(*net.UDPAddr); ok && ua != nil {
				port = ua.Port
			}
		}
		a.mu.Unlock()
		if port > 0 {
			return port
		}
		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func itoaPort(n int) string {
	return strconv.Itoa(n)
}
