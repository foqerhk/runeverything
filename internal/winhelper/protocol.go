package winhelper

// Named pipe used by the LocalSystem helper service.
const PipeName = `\\.\pipe\runeverything-helper`

// ServiceName is the Windows SCM service name.
const ServiceName = "RunEverythingHelper"

// Request is a JSON line from agent/smoke → helper.
type Request struct {
	Op      string  `json:"op"`
	MaxW    int     `json:"max_w,omitempty"`
	MaxH    int     `json:"max_h,omitempty"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	Buttons int     `json:"buttons,omitempty"`
	Down    bool    `json:"down,omitempty"`
	Delta   int     `json:"delta,omitempty"`
	KeyCode int     `json:"key_code,omitempty"`
	Text    string  `json:"text,omitempty"`
	Mods    int     `json:"modifiers,omitempty"`
}

// Response is a JSON line helper → client.
type Response struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	V       int    `json:"v,omitempty"`
	Desktop string `json:"desktop,omitempty"`
	Secure  bool   `json:"secure,omitempty"`
	W       int    `json:"w,omitempty"`
	H       int    `json:"h,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	System  bool   `json:"system,omitempty"`
	// RGBA is raw pixel bytes (encoding/json uses base64).
	RGBA []byte `json:"rgba,omitempty"`
}
