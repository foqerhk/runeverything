package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/reudp"
	"github.com/foqerhk/runeverything/internal/tunnel"
)

// retest: RE2.1 client — wss signaling + optional UDP Noise/desktop smoke.
func main() {
	relay := flag.String("relay", "ws://127.0.0.1:8787/re2", "relay wss URL")
	udp := flag.String("udp", "", "REUDP host:port (default: derived from relay)")
	device := flag.String("device", "", "device_id")
	token := flag.String("token", "", "pairing_token")
	sessionToken := flag.String("session", "", "existing session_ticket (skip pair)")
	noisePub := flag.String("noise-pub", "", "agent noise_pub from QR (base64url)")
	cwd := flag.String("cwd", "", "PTY cwd (legacy shell mode)")
	desktopMode := flag.Bool("desktop", true, "open desktop session (default); false = PTY")
	outFrame := flag.String("out-frame", "", "write first video annex-B to this path")
	flag.Parse()

	runClient(*relay, *udp, *device, *token, *sessionToken, *noisePub, *cwd, *desktopMode, *outFrame)
}

func runClient(relay, udpHost, device, token, sessionTicket, noisePubB64, cwd string, desktopMode bool, outFrame string) {
	relay = re2.EnsurePath(relay)
	u, err := url.Parse(relay)
	if err != nil {
		log.Fatal(err)
	}
	q := u.Query()
	q.Set("role", "client")
	u.RawQuery = q.Encode()

	tc, _, err := tunnel.Dial(u.String(), nil)
	if err != nil {
		log.Fatal(err)
	}
	c := re2.WrapWS(tc.WS)
	defer c.Close()

	dev := device
	ticket := sessionTicket
	if ticket == "" {
		if device == "" || token == "" {
			log.Fatal("need -device and -token, or -session")
		}
		if err := c.WriteFrame(re2.Frame{
			Type:    re2.TypePairRedeem,
			RouteID: device,
			Payload: re2.MustJSON(re2.PairRedeemPayload{DeviceID: device, PairingToken: token}),
		}); err != nil {
			log.Fatal(err)
		}
		f := mustReadRE2(c)
		if f.Type == re2.TypeError {
			log.Fatalf("pair failed: %s", string(f.Payload))
		}
		if f.Type != re2.TypePairAck {
			log.Fatalf("pair failed: type=%s", re2.FrameTypeName(f.Type))
		}
		var ack re2.PairAckPayload
		if err := json.Unmarshal(f.Payload, &ack); err != nil {
			log.Fatal(err)
		}
		ticket = ack.SessionTicket
		dev = ack.DeviceID
		fmt.Printf("paired session_ticket=%s name=%s\n", ticket, ack.Name)
	}

	if err := c.WriteFrame(re2.Frame{
		Type:    re2.TypeBind,
		RouteID: dev,
		Payload: re2.MustJSON(re2.BindPayload{DeviceID: dev, SessionTicket: ticket}),
	}); err != nil {
		log.Fatal(err)
	}
	f := mustReadRE2(c)
	if f.Type == re2.TypeError {
		log.Fatalf("bind failed: %s", string(f.Payload))
	}
	if f.Type != re2.TypeBindOK {
		log.Fatalf("bind failed: type=%s", re2.FrameTypeName(f.Type))
	}
	var bok re2.BindOKPayload
	_ = json.Unmarshal(f.Payload, &bok)
	fmt.Fprintln(os.Stderr, "[bind ok]")

	if udpHost == "" {
		udpHost = bok.UDP
	}
	if udpHost == "" {
		udpHost = udpFromURL(relay)
	}

	psk := re2.DerivePSK(token)
	if token == "" {
		log.Fatal("pairing token required for Noise PSK (pass -token even with -session)")
	}

	var sess *re2.Session
	var peerPub []byte
	var ep *reudp.Endpoint

	if udpHost != "" {
		ep, err = reudp.Dial(udpHost)
		if err != nil {
			log.Fatalf("udp dial: %v", err)
		}
		defer ep.Close()
		if err := ep.AssocClient(dev, ticket); err != nil {
			log.Fatal(err)
		}
		if _, err := ep.WaitAssocOK(10 * time.Second); err != nil {
			log.Fatalf("udp assoc: %v", err)
		}
		fmt.Fprintln(os.Stderr, "[reudp assoc ok]")
		hs, err := re2.NewClientHandshake(psk)
		if err != nil {
			log.Fatal(err)
		}
		tr := &reudp.NoiseTransport{EP: ep}
		sess, peerPub, err = hs.RunClient(tr)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "[warn: no udp; noise over wss]")
		hs, err := re2.NewClientHandshake(psk)
		if err != nil {
			log.Fatal(err)
		}
		tr := &re2.FrameNoiseTransport{Conn: c, RouteID: dev}
		sess, peerPub, err = hs.RunClient(tr)
		if err != nil {
			log.Fatal(err)
		}
	}

	if noisePubB64 != "" {
		want, err := base64.RawURLEncoding.DecodeString(noisePubB64)
		if err != nil || len(want) != 32 {
			log.Fatal("invalid -noise-pub")
		}
		if !bytes.Equal(want, peerPub) {
			log.Fatal("noise_pub pin mismatch")
		}
		fmt.Fprintln(os.Stderr, "[noise pin ok]")
	}

	sid := fmt.Sprintf("%d", time.Now().UnixNano())
	if len(sid) > 16 {
		sid = sid[len(sid)-16:]
	}

	sendInner := func(mt byte, body []byte, reliable bool) error {
		ct, err := sess.Encrypt(re2.EncodeInner(mt, body))
		if err != nil {
			return err
		}
		if ep != nil {
			if reliable {
				return ep.SendReliable(ct)
			}
			return ep.SendUnreliable(ct)
		}
		return c.WriteFrame(re2.Frame{Type: re2.TypeTunnel, RouteID: dev, Payload: ct})
	}

	recvInner := func() (byte, []byte, error) {
		var ct []byte
		var err error
		if ep != nil {
			ct, err = ep.RecvTimeout(30 * time.Second)
		} else {
			f, err2 := c.ReadFrame()
			if err2 != nil {
				return 0, nil, err2
			}
			if f.Type != re2.TypeTunnel {
				return 0, nil, fmt.Errorf("unexpected type %s", re2.FrameTypeName(f.Type))
			}
			ct = f.Payload
		}
		if err != nil {
			return 0, nil, err
		}
		plain, err := sess.Decrypt(ct)
		if err != nil {
			return 0, nil, err
		}
		return re2.DecodeInner(plain)
	}

	if desktopMode {
		if err := sendInner(re2.MsgOpenDesktop, re2.MustJSON(re2.OpenDesktopPayload{
			SessionID: sid,
			MaxWidth:  1280,
			MaxHeight: 720,
			FPS:       5,
			Codec:     "h264",
		}), true); err != nil {
			log.Fatal(err)
		}
		for {
			mt, body, err := recvInner()
			if err != nil {
				log.Fatal(err)
			}
			switch mt {
			case re2.MsgDesktopReady:
				fmt.Fprintln(os.Stderr, "[desktop ready]", string(body))
			case re2.MsgVideo:
				_, fid, flags, part, parts, nal, err := re2.DecodeVideo(body)
				if err != nil {
					continue
				}
				fmt.Fprintf(os.Stderr, "[video] frame=%d flags=%d part=%d/%d nal=%d\n", fid, flags, part+1, parts, len(nal))
				if outFrame != "" && part == 0 {
					_ = os.WriteFile(outFrame, nal, 0o644)
					fmt.Fprintln(os.Stderr, "[wrote]", outFrame)
				}
				// send a click
				_ = sendInner(re2.MsgInputMouse, re2.MustJSON(re2.InputMousePayload{
					SessionID: sid, X: 0.5, Y: 0.5, Buttons: 1, Down: true, Move: true,
				}), true)
				_ = sendInner(re2.MsgInputMouse, re2.MustJSON(re2.InputMousePayload{
					SessionID: sid, X: 0.5, Y: 0.5, Buttons: 1, Up: true,
				}), true)
				fmt.Fprintln(os.Stderr, "REUDP_DESKTOP_SMOKE_OK")
				return
			case re2.MsgAppError:
				log.Fatal(string(body))
			}
		}
	}

	// PTY mode
	if err := sendInner(re2.MsgOpenSession, re2.MustJSON(re2.OpenSessionPayload{
		SessionID: sid, Cwd: cwd, Cols: 80, Rows: 24,
	}), true); err != nil {
		log.Fatal(err)
	}
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				body := re2.EncodePTY(sid, buf[:n])
				_ = sendInner(re2.MsgPTYData, body, true)
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		mt, body, err := recvInner()
		if err != nil {
			log.Fatal(err)
		}
		switch mt {
		case re2.MsgSessionReady:
			fmt.Fprintln(os.Stderr, "[session ready]")
		case re2.MsgPTYData:
			_, data, err := re2.DecodePTY(body)
			if err != nil {
				continue
			}
			_, _ = os.Stdout.Write(data)
		case re2.MsgSessionClose:
			return
		case re2.MsgAppError:
			fmt.Fprintln(os.Stderr, string(body))
		}
	}
}

func mustReadRE2(c *re2.Conn) *re2.Frame {
	_ = c.WS.SetReadDeadline(time.Now().Add(15 * time.Second))
	f, err := c.ReadFrame()
	_ = c.WS.SetReadDeadline(time.Time{})
	if err != nil {
		log.Fatal(err)
	}
	return f
}

func udpFromURL(relay string) string {
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
