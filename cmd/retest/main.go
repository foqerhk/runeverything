package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/foqerhk/runeverything/internal/protocol"
	"github.com/foqerhk/runeverything/internal/tunnel"
)

// retest is a minimal client for local smoke tests (pair + open shell session).
func main() {
	relay := flag.String("relay", "ws://127.0.0.1:8787/ws", "relay URL")
	device := flag.String("device", "", "device_id")
	token := flag.String("token", "", "pairing_token")
	sessionToken := flag.String("session", "", "existing session_token (skip pair)")
	cwd := flag.String("cwd", "", "session cwd")
	flag.Parse()

	u, err := url.Parse(*relay)
	if err != nil {
		log.Fatal(err)
	}
	q := u.Query()
	q.Set("role", protocol.RoleClient)
	u.RawQuery = q.Encode()

	c, _, err := tunnel.Dial(u.String(), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	sess := *sessionToken
	dev := *device
	if sess == "" {
		if *device == "" || *token == "" {
			log.Fatal("need -device and -token, or -session")
		}
		b, _ := protocol.Encode(protocol.TypePairRedeem, "1", protocol.PairRedeemData{
			DeviceID:     *device,
			PairingToken: *token,
		})
		if err := c.WriteText(b); err != nil {
			log.Fatal(err)
		}
		env := mustRead(c)
		if env.Type != protocol.TypePairAck {
			log.Fatalf("pair failed: %+v", dump(env))
		}
		var ack protocol.PairAckData
		_ = protocol.DecodeData(env, &ack)
		sess = ack.SessionToken
		dev = ack.DeviceID
		fmt.Printf("paired session_token=%s name=%s\n", sess, ack.Name)
	}

	b, _ := protocol.Encode(protocol.TypeClientHello, "2", protocol.ClientHelloData{
		DeviceID:     dev,
		SessionToken: sess,
	})
	if err := c.WriteText(b); err != nil {
		log.Fatal(err)
	}
	env := mustRead(c)
	if env.Type != protocol.TypeClientOK {
		log.Fatalf("hello failed: %+v", dump(env))
	}
	var okd protocol.ClientOKData
	_ = protocol.DecodeData(env, &okd)
	fmt.Printf("client_ok online=%v\n", okd.Online)

	sid := fmt.Sprintf("%d", time.Now().UnixNano())
	if len(sid) > 16 {
		sid = sid[len(sid)-16:]
	}
	b, _ = protocol.Encode(protocol.TypeSessionOpen, "3", protocol.SessionOpenData{
		SessionID: sid,
		Cwd:       *cwd,
		Cols:      80,
		Rows:      24,
	})
	if err := c.WriteText(b); err != nil {
		log.Fatal(err)
	}

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				frame := protocol.EncodePTYFrame(protocol.DirStdin, sid, buf[:n])
				_ = c.WriteBinary(frame)
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		mt, msg, err := c.WS.ReadMessage()
		if err != nil {
			log.Fatal(err)
		}
		if mt == websocket.BinaryMessage {
			_, _, payload, err := protocol.DecodePTYFrame(msg)
			if err != nil {
				continue
			}
			_, _ = os.Stdout.Write(payload)
			continue
		}
		env, err := protocol.Decode(msg)
		if err != nil {
			continue
		}
		switch env.Type {
		case protocol.TypeSessionReady:
			fmt.Fprintln(os.Stderr, "[session ready]")
		case protocol.TypeSessionClose:
			fmt.Fprintln(os.Stderr, "[session closed]")
			return
		case protocol.TypeError:
			fmt.Fprintln(os.Stderr, dump(env))
		}
	}
}

func mustRead(c *tunnel.Conn) *protocol.Envelope {
	_ = c.WS.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, msg, err := c.WS.ReadMessage()
	if err != nil {
		log.Fatal(err)
	}
	env, err := protocol.Decode(msg)
	if err != nil {
		log.Fatal(err)
	}
	return env
}

func dump(env *protocol.Envelope) string {
	b, _ := json.Marshal(env)
	return string(b)
}

var _ = io.EOF
