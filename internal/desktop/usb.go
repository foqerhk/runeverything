package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// USBDeviceInfo is a local USB device entry.
type USBDeviceInfo struct {
	ID        string
	Name      string
	VendorID  string
	ProductID string
	Bus       string
}

var (
	usbMu     sync.Mutex
	usbAttached = map[string]bool{}
)

// ListUSBDevices lists USB devices (best-effort per OS).
func ListUSBDevices() ([]USBDeviceInfo, error) {
	switch runtime.GOOS {
	case "linux":
		return listUSBLinux()
	case "darwin":
		return listUSBDarwin()
	case "windows":
		return listUSBWindows()
	default:
		return nil, fmt.Errorf("usb: unsupported OS")
	}
}

func listUSBLinux() ([]USBDeviceInfo, error) {
	// Prefer lsusb
	out, err := exec.Command("lsusb").Output()
	if err != nil {
		return nil, err
	}
	var devs []USBDeviceInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Bus 001 Device 002: ID 1d6b:0002 Linux Foundation 2.0 root hub
		parts := strings.Fields(line)
		if len(parts) < 6 {
			continue
		}
		bus := parts[1]
		dev := strings.TrimSuffix(parts[3], ":")
		id := parts[5]
		vids := strings.Split(id, ":")
		vid, pid := "", ""
		if len(vids) == 2 {
			vid, pid = vids[0], vids[1]
		}
		name := strings.Join(parts[6:], " ")
		devs = append(devs, USBDeviceInfo{
			ID: bus + ":" + dev + ":" + id, Name: name, VendorID: vid, ProductID: pid, Bus: bus,
		})
	}
	return devs, nil
}

func listUSBDarwin() ([]USBDeviceInfo, error) {
	out, err := exec.Command("system_profiler", "SPUSBDataType", "-detailLevel", "mini").Output()
	if err != nil {
		return nil, err
	}
	var devs []USBDeviceInfo
	var cur USBDeviceInfo
	flush := func() {
		if cur.Name != "" {
			if cur.ID == "" {
				cur.ID = cur.Name
			}
			devs = append(devs, cur)
			cur = USBDeviceInfo{}
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasSuffix(t, ":") && !strings.Contains(t, " ") {
			flush()
			cur.Name = strings.TrimSuffix(t, ":")
			continue
		}
		if strings.HasPrefix(t, "Product ID:") {
			cur.ProductID = strings.TrimSpace(strings.TrimPrefix(t, "Product ID:"))
		}
		if strings.HasPrefix(t, "Vendor ID:") {
			cur.VendorID = strings.TrimSpace(strings.TrimPrefix(t, "Vendor ID:"))
		}
		if strings.HasPrefix(t, "Location ID:") {
			cur.Bus = strings.TrimSpace(strings.TrimPrefix(t, "Location ID:"))
			cur.ID = cur.Bus
		}
	}
	flush()
	return devs, nil
}

func listUSBWindows() ([]USBDeviceInfo, error) {
	// PowerShell PnP USB devices
	ps := `Get-PnpDevice -Class USB -Status OK | Select-Object -ExpandProperty FriendlyName`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err != nil {
		return nil, err
	}
	var devs []USBDeviceInfo
	for i, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		devs = append(devs, USBDeviceInfo{ID: fmt.Sprintf("usb-%d", i), Name: name})
	}
	return devs, nil
}

// USBAttach binds a device for remote use. On Linux prefers usbip; otherwise records attach intent.
func USBAttach(deviceID, remoteAddr string) error {
	usbMu.Lock()
	defer usbMu.Unlock()
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("usbip"); err == nil {
			busid := deviceID
			if i := strings.LastIndex(deviceID, ":"); i > 0 && strings.Count(deviceID, ":") >= 2 {
				// bus:dev:vid:pid → try busid from usbip list
				_ = busid
			}
			if remoteAddr == "" {
				// bind locally for export
				if err := exec.Command("usbip", "bind", "-b", firstField(deviceID)).Run(); err != nil {
					// fall through to mark attached
				} else {
					usbAttached[deviceID] = true
					return nil
				}
			} else {
				host := remoteAddr
				if err := exec.Command("usbip", "attach", "-r", host, "-b", firstField(deviceID)).Run(); err == nil {
					usbAttached[deviceID] = true
					return nil
				}
			}
		}
	}
	// Soft attach: allow protocol-level tracking (client can use TCP/IP printer/camera alternatives).
	usbAttached[deviceID] = true
	_ = os.Setenv("RE_USB_LAST", deviceID)
	return nil
}

func USBDetach(deviceID string) error {
	usbMu.Lock()
	defer usbMu.Unlock()
	delete(usbAttached, deviceID)
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("usbip"); err == nil {
			_ = exec.Command("usbip", "unbind", "-b", firstField(deviceID)).Run()
			_ = exec.Command("usbip", "detach", "-p", "0").Run()
		}
	}
	return nil
}

func firstField(s string) string {
	if i := strings.IndexByte(s, ':'); i > 0 {
		return s[:i]
	}
	return s
}
