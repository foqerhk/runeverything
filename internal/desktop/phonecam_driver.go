package desktop

// PhoneCamDriverStatus describes the optional host virtual-webcam driver
// used by “Use Phone as Webcam” (AkVirtualCamera / OBS Virtual Camera, etc.).
type PhoneCamDriverStatus struct {
	Installed bool
	Name      string // e.g. "AkVirtualCamera", "OBS Virtual Camera"
}

// PhoneCamDriver reports whether a usable phone→PC virtual webcam driver is present.
func PhoneCamDriver() PhoneCamDriverStatus {
	return phoneCamDriverStatus()
}

// OpenPhoneCamDriverInstall runs the bundled AkVirtualCamera installer shipped
// with the Agent (macOS .pkg / Windows .exe). The OS prompts for an admin
// password / UAC — it does not open a browser.
func OpenPhoneCamDriverInstall() error {
	return openPhoneCamDriverInstall()
}
