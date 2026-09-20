package re2

import "encoding/json"

// Control JSON payloads for outer frames (relay-visible plaintext).

type RegisterPayload struct {
	DeviceID     string `json:"device_id"`
	DeviceSecret string `json:"device_secret"`
	Name         string `json:"name"`
	OS           string `json:"os,omitempty"`
	Arch         string `json:"arch,omitempty"`
	NoisePub     string `json:"noise_pub,omitempty"`
}

type RegisterOKPayload struct {
	DeviceID string `json:"device_id"`
	UDP      string `json:"udp,omitempty"`
}

type PairOfferPayload struct {
	DeviceID     string `json:"device_id"`
	PairingToken string `json:"pairing_token"`
	Name         string `json:"name"`
	ExpiresAt    int64  `json:"expires_at"`
	NoisePub     string `json:"noise_pub,omitempty"`
}

type PairRedeemPayload struct {
	DeviceID       string `json:"device_id"`
	PairingToken   string `json:"pairing_token"`
	ClientNoisePub string `json:"client_noise_pub,omitempty"`
}

type PairAckPayload struct {
	DeviceID      string `json:"device_id"`
	SessionTicket string `json:"session_ticket"`
	Name          string `json:"name"`
}

type BindPayload struct {
	DeviceID      string `json:"device_id"`
	SessionTicket string `json:"session_ticket"`
}

type BindOKPayload struct {
	OK  bool   `json:"ok"`
	UDP string `json:"udp,omitempty"` // host:port for REUDP data plane
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type OpenSessionPayload struct {
	SessionID string   `json:"session_id"`
	Cols      int      `json:"cols,omitempty"`
	Rows      int      `json:"rows,omitempty"`
	Cmd       []string `json:"cmd,omitempty"`
	Cwd       string   `json:"cwd,omitempty"`
	UseTmux   bool     `json:"use_tmux,omitempty"`
	TmuxName  string   `json:"tmux_name,omitempty"`
}

type SessionReadyPayload struct {
	SessionID string `json:"session_id"`
}

type SessionClosePayload struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

type ResizePayload struct {
	SessionID string `json:"session_id"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// Desktop session payloads (RE2.1).

type OpenDesktopPayload struct {
	SessionID   string `json:"session_id"`
	MaxWidth    int    `json:"max_width,omitempty"`
	MaxHeight   int    `json:"max_height,omitempty"`
	FPS         int    `json:"fps,omitempty"`
	Codec       string `json:"codec,omitempty"` // "h264"
	DisplayID   int    `json:"display_id,omitempty"`
	BitrateKbps int    `json:"bitrate_kbps,omitempty"`
	HideCursor  bool   `json:"hide_cursor,omitempty"` // capture without OS cursor baked in
	Password    string `json:"password,omitempty"`    // optional; checked against RE_ACCESS_PASSWORD
	PrivacyBlank bool  `json:"privacy_blank,omitempty"` // cover local screen (exclude from capture when possible)
}

type DesktopReadyPayload struct {
	SessionID   string          `json:"session_id"`
	Width       int             `json:"width"`
	Height      int             `json:"height"`
	Codec       string          `json:"codec"`
	FPS         int             `json:"fps,omitempty"`
	BitrateKbps int             `json:"bitrate_kbps,omitempty"`
	DisplayID   int             `json:"display_id,omitempty"`
	Displays    []DisplayInfo   `json:"displays,omitempty"`
}

type DisplayInfo struct {
	ID     int    `json:"id"`
	Name   string `json:"name,omitempty"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	X      int    `json:"x,omitempty"`
	Y      int    `json:"y,omitempty"`
	Primary bool  `json:"primary,omitempty"`
}

type DisplaysPayload struct {
	SessionID string        `json:"session_id,omitempty"`
	Action    string        `json:"action"` // list|select
	DisplayID int           `json:"display_id,omitempty"`
	Displays  []DisplayInfo `json:"displays,omitempty"`
}

type CursorPayload struct {
	SessionID string  `json:"session_id"`
	X         float64 `json:"x"` // normalized on selected display
	Y         float64 `json:"y"`
	Visible   bool    `json:"visible"`
	HotX      int     `json:"hot_x,omitempty"`
	HotY      int     `json:"hot_y,omitempty"`
}

type ClipboardPayload struct {
	SessionID string `json:"session_id"`
	Mime      string `json:"mime"` // text/plain, image/png, …
	Text      string `json:"text,omitempty"`
	DataB64   string `json:"data_b64,omitempty"`
}

type AudioPayload struct {
	SessionID string `json:"session_id"`
	Codec     string `json:"codec"` // opus|pcm16
	SampleRate int   `json:"sample_rate,omitempty"`
	Channels  int    `json:"channels,omitempty"`
	DataB64   string `json:"data_b64"`
}

type StatsPayload struct {
	SessionID   string  `json:"session_id"`
	RTTMs       int     `json:"rtt_ms"`
	LossPct     float64 `json:"loss_pct"`
	RecvKbps    int     `json:"recv_kbps,omitempty"`
	DecodeFPS   float64 `json:"decode_fps,omitempty"`
	WantKeyframe bool   `json:"want_keyframe,omitempty"`
}

type KeyframeReqPayload struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

type FileOfferPayload struct {
	SessionID string `json:"session_id"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Mime      string `json:"mime,omitempty"`
}

type FileChunkPayload struct {
	SessionID string `json:"session_id"`
	FileID    string `json:"file_id"`
	Offset    int64  `json:"offset"`
	DataB64   string `json:"data_b64"`
	EOF       bool   `json:"eof,omitempty"`
}

type HolePunchPayload struct {
	SessionID string `json:"session_id,omitempty"`
	Action    string `json:"action"` // offer|answer|candidate|connected|failed
	UDPAddr   string `json:"udp_addr,omitempty"` // host:port reflexive/local
	Token     string `json:"token,omitempty"`
	Candidates []string `json:"candidates,omitempty"` // host:port list for ICE-lite
}

type PairConfirmPayload struct {
	DeviceID string `json:"device_id"`
	ClientName string `json:"client_name,omitempty"`
	Approved bool   `json:"approved"`
}

type AuditPayload struct {
	TimeUnix int64  `json:"t"`
	Event    string `json:"event"`
	Detail   string `json:"detail,omitempty"`
}

type DesktopClosePayload struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

type InputMousePayload struct {
	SessionID string  `json:"session_id"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Buttons   int     `json:"buttons,omitempty"`
	Wheel     int     `json:"wheel,omitempty"`
	WheelH    int     `json:"wheel_h,omitempty"`
	Down      bool    `json:"down,omitempty"`
	Up        bool    `json:"up,omitempty"`
	Move      bool    `json:"move,omitempty"`
	Relative  bool    `json:"relative,omitempty"` // game mode: DX/DY in pixels
	DX        float64 `json:"dx,omitempty"`
	DY        float64 `json:"dy,omitempty"`
}

type InputKeyPayload struct {
	SessionID string `json:"session_id"`
	KeyCode   int    `json:"key_code,omitempty"`
	Text      string `json:"text,omitempty"` // IME committed text
	Down      bool   `json:"down"`
	Modifiers int    `json:"modifiers,omitempty"` // bit0 shift bit1 ctrl bit2 alt bit3 meta
	Repeat    bool   `json:"repeat,omitempty"`
}

type InputTouchPayload struct {
	SessionID string  `json:"session_id"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Phase     string  `json:"phase"`
	ID        int     `json:"id,omitempty"`
}

type InputModePayload struct {
	SessionID    string `json:"session_id"`
	RelativeMouse bool  `json:"relative_mouse,omitempty"`
	GameMode     bool   `json:"game_mode,omitempty"`
	CapsLock     *bool  `json:"caps_lock,omitempty"`
	NumLock      *bool  `json:"num_lock,omitempty"`
	ScrollLock   *bool  `json:"scroll_lock,omitempty"`
}

type FileListPayload struct {
	SessionID string         `json:"session_id"`
	Path      string         `json:"path,omitempty"` // relative to xfer root
	Entries   []FileListEntry `json:"entries,omitempty"`
	Error     string         `json:"error,omitempty"`
}

type FileListEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
	ModUnix int64 `json:"mod_unix,omitempty"`
}

type FileAckPayload struct {
	SessionID string  `json:"session_id"`
	FileID    string  `json:"file_id"`
	Offset    int64   `json:"offset"`
	OK        bool    `json:"ok"`
	Error     string  `json:"error,omitempty"`
	Progress  float64 `json:"progress,omitempty"` // 0..1
	ResumeFrom int64  `json:"resume_from,omitempty"`
}

// FilePullPayload: client asks agent to push a file from the agent machine.
type FilePullPayload struct {
	SessionID  string `json:"session_id"`
	FileID     string `json:"file_id"`
	Path       string `json:"path"`
	ResumeFrom int64  `json:"resume_from,omitempty"`
}

type WakeOnLANPayload struct {
	SessionID string `json:"session_id,omitempty"`
	MAC       string `json:"mac"`                 // AA:BB:CC:DD:EE:FF
	Broadcast string `json:"broadcast,omitempty"` // default 255.255.255.255:9
}

type CameraListPayload struct {
	SessionID string         `json:"session_id,omitempty"`
	Devices   []CameraDevice `json:"devices,omitempty"`
}

type CameraDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CameraOpenPayload struct {
	SessionID string `json:"session_id"`
	DeviceID  string `json:"device_id,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	FPS       int    `json:"fps,omitempty"`
	Codec     string `json:"codec,omitempty"` // mjpeg|h264
}

type CameraClosePayload struct {
	SessionID string `json:"session_id"`
}

type CameraFramePayload struct {
	SessionID string `json:"session_id"`
	Codec     string `json:"codec"` // mjpeg|h264
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	DataB64   string `json:"data_b64"`
	KeyFrame  bool   `json:"key_frame,omitempty"`
}

type USBListPayload struct {
	SessionID string      `json:"session_id,omitempty"`
	Devices   []USBDevice `json:"devices,omitempty"`
	Error     string      `json:"error,omitempty"`
}

type USBDevice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	VendorID string `json:"vendor_id,omitempty"`
	ProductID string `json:"product_id,omitempty"`
	Bus      string `json:"bus,omitempty"`
}

type USBAttachPayload struct {
	SessionID string `json:"session_id"`
	DeviceID  string `json:"device_id"`
	RemoteAddr string `json:"remote_addr,omitempty"` // usbip host:port when applicable
}

type USBDetachPayload struct {
	SessionID string `json:"session_id"`
	DeviceID  string `json:"device_id"`
}

type USBDataPayload struct {
	SessionID string `json:"session_id"`
	DeviceID  string `json:"device_id"`
	DataB64   string `json:"data_b64"`
}

type PrinterListPayload struct {
	SessionID string          `json:"session_id,omitempty"`
	Printers  []PrinterInfo   `json:"printers,omitempty"`
}

type PrinterInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Default bool   `json:"default,omitempty"`
}

type PrinterJobPayload struct {
	SessionID string `json:"session_id"`
	JobID     string `json:"job_id"`
	PrinterID string `json:"printer_id"`
	Name      string `json:"name,omitempty"`
	Mime      string `json:"mime,omitempty"` // application/pdf, application/octet-stream
	DataB64   string `json:"data_b64"`
}

type PrinterAckPayload struct {
	SessionID string `json:"session_id"`
	JobID     string `json:"job_id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

func MustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func FrameTypeName(t byte) string {
	switch t {
	case TypeRegister:
		return "REGISTER"
	case TypeRegisterOK:
		return "REGISTER_OK"
	case TypePairOffer:
		return "PAIR_OFFER"
	case TypePairRedeem:
		return "PAIR_REDEEM"
	case TypePairAck:
		return "PAIR_ACK"
	case TypeBind:
		return "BIND"
	case TypeBindOK:
		return "BIND_OK"
	case TypeNoise:
		return "NOISE"
	case TypeTunnel:
		return "TUNNEL"
	case TypePing:
		return "PING"
	case TypePong:
		return "PONG"
	case TypeError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
