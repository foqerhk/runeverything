package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/audit"
	"github.com/foqerhk/runeverything/internal/desktop"
	"github.com/foqerhk/runeverything/internal/re2"
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
		return a.sendTunnel(re2.MsgFileList, re2.MustJSON(re2.FileListPayload{SessionID: p.SessionID, Error: "path not allowed"}), true)
	}
	entries, err := os.ReadDir(target)
	out := re2.FileListPayload{SessionID: p.SessionID, Path: rel}
	if err != nil {
		out.Error = err.Error()
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
	return a.sendTunnel(re2.MsgFileList, re2.MustJSON(out), true)
}

func (a *Agent) cameraPump(sid string, cam *desktop.CameraCapture, ctx context.Context) {
	defer a.closeCamera()
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
			w, h := cam.Size()
			_ = a.sendTunnel(re2.MsgCameraFrame, re2.MustJSON(re2.CameraFramePayload{
				SessionID: sid, Codec: cam.Codec(), Width: w, Height: h,
				DataB64: desktop.EncodeB64(frame), KeyFrame: cam.Codec() != "h264",
			}), false)
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

func localUDPCandidates() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil {
				continue
			}
			out = append(out, ip.String()+":0")
		}
	}
	return out
}
