package main

import (
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/foqerhk/runeverything/internal/i18n"
	"github.com/foqerhk/runeverything/internal/re2"
)

// The data channel carries AI session inventory and PTY traffic on its own Noise
// session, so the desktop session (UDP/WSS, liveness, rekeys) and the data
// channel never replace or block each other for the same phone.

const dataNoiseTimeout = 10 * time.Second

func (a *Agent) dataRoute() string {
	return re2.ChannelRoute(a.id.DeviceID, re2.ChannelData)
}

// chanNoiseTransport runs a handshake off the relay read loop: the loop feeds
// later data-channel Noise frames into ch.
type chanNoiseTransport struct {
	first   []byte
	ch      chan []byte
	conn    *re2.Conn
	routeID string
}

func (t *chanNoiseTransport) Send(msg []byte) error {
	return t.conn.WriteFrame(re2.Frame{Type: re2.TypeNoise, RouteID: t.routeID, Payload: msg})
}

func (t *chanNoiseTransport) Recv() ([]byte, error) {
	if t.first != nil {
		m := t.first
		t.first = nil
		return m, nil
	}
	select {
	case m := <-t.ch:
		return m, nil
	case <-time.After(dataNoiseTimeout):
		return nil, errors.New("re2: data channel handshake timed out")
	}
}

// handleDataFrame consumes frames routed to the data channel and reports
// whether f was one.
func (a *Agent) handleDataFrame(f *re2.Frame) bool {
	if _, ch := re2.SplitRoute(f.RouteID); ch != re2.ChannelData {
		return false
	}
	switch f.Type {
	case re2.TypeNoise:
		a.onDataNoise(f.Payload)
	case re2.TypeTunnel:
		a.onDataTunnel(f.Payload)
	case re2.TypeError:
		var ed re2.ErrorPayload
		_ = json.Unmarshal(f.Payload, &ed)
		i18n.Log("log.re2_relay_error", ed.Code, ed.Message)
		if ed.Code == "peer_gone" {
			a.dropDataChannel("peer_gone")
		}
	}
	return true
}

func (a *Agent) onDataNoise(payload []byte) {
	a.mu.Lock()
	if ch := a.dataNoise; ch != nil {
		done := a.dataNoiseDone
		a.mu.Unlock()
		select {
		case ch <- payload:
		default:
		}
		// The phone sends its first TUNNEL right after msg3; hold the read loop
		// until the session is installed so that frame is not dropped.
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		return
	}
	ready := a.pairingToken != ""
	a.mu.Unlock()
	if !ready {
		log.Printf("re2 data NOISE before pair offer; ignoring")
		return
	}
	// A new msg1 means the phone's previous data tunnel is gone.
	a.dropDataChannel("rehandshake")

	ch := make(chan []byte, 4)
	done := make(chan struct{})
	a.mu.Lock()
	a.dataNoise = ch
	a.dataNoiseDone = done
	conn := a.re2Conn
	a.mu.Unlock()
	go func() {
		defer close(done)
		tr := &chanNoiseTransport{first: payload, ch: ch, conn: conn, routeID: a.dataRoute()}
		sess, _, _, err := a.runAgentNoise(tr)
		a.mu.Lock()
		current := a.dataNoise == ch
		if current {
			a.dataNoise = nil
			if err == nil {
				a.dataSess = sess
			}
		}
		a.mu.Unlock()
		if err != nil {
			i18n.Log("log.re2_handshake_fail", err)
			return
		}
		if current {
			log.Printf("re2 data channel ready device=%s", a.id.DeviceID)
		}
	}()
}

func (a *Agent) onDataTunnel(payload []byte) {
	a.mu.Lock()
	sess := a.dataSess
	a.mu.Unlock()
	if sess == nil {
		log.Printf("re2 data TUNNEL before handshake device=%s len=%d", a.id.DeviceID, len(payload))
		return
	}
	plain, err := sess.Decrypt(payload)
	if err != nil {
		i18n.Log("log.re2_decrypt_fail", err)
		return
	}
	if err := a.handleDataInner(plain); err != nil {
		i18n.Log("log.re2_inner", err)
	}
}

// handleDataInner serves the data-only subset of inner messages; desktop,
// input and peripheral messages belong to the desktop channel.
func (a *Agent) handleDataInner(plain []byte) error {
	mt, body, err := re2.DecodeInner(plain)
	if err != nil {
		return err
	}
	switch mt {
	case re2.MsgPing:
		return a.sendData(re2.MsgPong, body)

	case re2.MsgPong:

	case re2.MsgOpenSession:
		var data re2.OpenSessionPayload
		if err := json.Unmarshal(body, &data); err != nil {
			return a.sendDataAppErr("bad_data", err.Error())
		}
		if err := a.openSessionOn(data, re2.ChannelData); err != nil {
			return a.sendDataAppErr("session_open_failed", err.Error())
		}
		return a.sendData(re2.MsgSessionReady, re2.MustJSON(re2.SessionReadyPayload{SessionID: data.SessionID}))

	case re2.MsgSessionClose:
		var data re2.SessionClosePayload
		_ = json.Unmarshal(body, &data)
		a.closeSession(data.SessionID, data.Reason)

	case re2.MsgResize:
		var data re2.ResizePayload
		_ = json.Unmarshal(body, &data)
		a.mu.Lock()
		s := a.sessions[data.SessionID]
		a.mu.Unlock()
		if s != nil {
			_ = s.Resize(data.Cols, data.Rows)
		}

	case re2.MsgPTYData:
		sid, data, err := re2.DecodePTY(body)
		if err != nil {
			return err
		}
		a.mu.Lock()
		s := a.sessions[sid]
		a.mu.Unlock()
		if s != nil {
			_, _ = s.Write(data)
		}

	case re2.MsgAgentChatList, re2.MsgAgentChatDetail:
		bodyCopy := append([]byte(nil), body...)
		go func() {
			if err := a.handleAgentChat(mt, bodyCopy, a.sendData); err != nil {
				log.Printf("agent chat (data channel): %v", err)
			}
		}()

	default:
		log.Printf("re2 data channel: ignoring inner type %d", mt)
	}
	return nil
}

func (a *Agent) sendData(msgType byte, body []byte) error {
	a.dataSendMu.Lock()
	defer a.dataSendMu.Unlock()
	a.mu.Lock()
	sess := a.dataSess
	conn := a.re2Conn
	a.mu.Unlock()
	if sess == nil || conn == nil {
		return errNoSession
	}
	ct, err := sess.Encrypt(re2.EncodeInner(msgType, body))
	if err != nil {
		return err
	}
	return conn.WriteFrame(re2.Frame{Type: re2.TypeTunnel, RouteID: a.dataRoute(), Payload: ct})
}

func (a *Agent) sendDataAppErr(code, msg string) error {
	return a.sendData(re2.MsgAppError, re2.MustJSON(re2.ErrorPayload{Code: code, Message: msg}))
}

// dropDataChannel forgets the data-channel Noise session and closes the PTYs it
// opened (AI CLIs live on in their screen sessions).
func (a *Agent) dropDataChannel(reason string) {
	a.mu.Lock()
	had := a.dataSess != nil || a.dataNoise != nil
	a.dataSess = nil
	a.dataNoise = nil
	var ids []string
	for id, ch := range a.sessionChan {
		if ch == re2.ChannelData {
			ids = append(ids, id)
		}
	}
	a.mu.Unlock()
	for _, id := range ids {
		a.closeSession(id, reason)
	}
	if had || len(ids) > 0 {
		log.Printf("re2 data channel closed (%s) ptys=%d device=%s", reason, len(ids), a.id.DeviceID)
	}
}

func (a *Agent) sessionOwner(id string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessionChan[id]
}
