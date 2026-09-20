package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/foqerhk/runeverything/internal/auth"
	"github.com/foqerhk/runeverything/internal/protocol"
	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/tunnel"
)

func handleRE2(hub *Hub, w http.ResponseWriter, r *http.Request) {
	ws, err := tunnel.DefaultUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("re2 upgrade: %v", err)
		return
	}
	c := re2.WrapWS(ws)
	defer c.Close()

	role := r.URL.Query().Get("role")
	switch role {
	case protocol.RoleAgent:
		serveRE2Agent(hub, c, r)
	case protocol.RoleClient:
		serveRE2Client(hub, c, r)
	default:
		_ = writeRE2Err(c, "", "bad_role", "query role=agent|client required")
	}
}

func writeRE2Err(c *re2.Conn, routeID, code, msg string) error {
	return c.WriteFrame(re2.Frame{
		Type:    re2.TypeError,
		RouteID: routeID,
		Payload: re2.MustJSON(re2.ErrorPayload{Code: code, Message: msg}),
	})
}

func logOpaque(typ byte, routeID string, payloadLen int) {
	log.Printf("re2 forward type=%s route=%s len=%d", re2.FrameTypeName(typ), routeID, payloadLen)
}

func (h *Hub) setRE2Agent(id, secret, name string, c *re2.Conn) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		d = &deviceState{ID: id}
		h.devices[id] = d
	}
	if d.Secret != "" && !auth.ConstantTimeEqual(d.Secret, secret) {
		return fmt.Errorf("invalid device secret")
	}
	if d.RE2Agent != nil && d.RE2Agent != c {
		_ = d.RE2Agent.Close()
	}
	d.Secret = secret
	d.Name = name
	d.RE2Agent = c
	return nil
}

func (h *Hub) clearRE2Agent(id string, c *re2.Conn) *re2.Conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		return nil
	}
	if d.RE2Agent != c {
		return nil
	}
	d.RE2Agent = nil
	peer := d.RE2Client
	d.RE2Client = nil
	return peer
}

func (h *Hub) re2AgentOf(id string) *re2.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	d := h.devices[id]
	if d == nil {
		return nil
	}
	return d.RE2Agent
}

func (h *Hub) setRE2Client(id string, c *re2.Conn) *re2.Conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		d = &deviceState{ID: id}
		h.devices[id] = d
	}
	old := d.RE2Client
	if old != nil && old != c {
		_ = old.Close()
	}
	d.RE2Client = c
	return old
}

func (h *Hub) clearRE2Client(id string, c *re2.Conn) *re2.Conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		return nil
	}
	if d.RE2Client != c {
		return nil
	}
	d.RE2Client = nil
	return d.RE2Agent
}

func (h *Hub) re2ClientOf(id string) *re2.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	d := h.devices[id]
	if d == nil {
		return nil
	}
	return d.RE2Client
}

func notifyPeerGone(peer *re2.Conn, routeID string) {
	if peer == nil {
		return
	}
	_ = writeRE2Err(peer, routeID, "peer_gone", "peer disconnected")
}

func serveRE2Agent(hub *Hub, c *re2.Conn, r *http.Request) {
	deviceID := ""
	defer func() {
		if deviceID != "" {
			peer := hub.clearRE2Agent(deviceID, c)
			notifyPeerGone(peer, deviceID)
		}
	}()

	for {
		f, err := c.ReadFrame()
		if err != nil {
			return
		}
		switch f.Type {
		case re2.TypeRegister:
			var data re2.RegisterPayload
			if err := json.Unmarshal(f.Payload, &data); err != nil {
				_ = writeRE2Err(c, f.RouteID, "bad_data", err.Error())
				continue
			}
			if data.DeviceID == "" || data.DeviceSecret == "" {
				_ = writeRE2Err(c, data.DeviceID, "bad_data", "device_id and device_secret required")
				continue
			}
			if err := hub.setRE2Agent(data.DeviceID, data.DeviceSecret, data.Name, c); err != nil {
				_ = writeRE2Err(c, data.DeviceID, "auth_failed", err.Error())
				return
			}
			deviceID = data.DeviceID
			_ = c.WriteFrame(re2.Frame{
				Type:    re2.TypeRegisterOK,
				RouteID: deviceID,
				Payload: re2.MustJSON(re2.RegisterOKPayload{DeviceID: deviceID, UDP: hub.publicUDP}),
			})
			log.Printf("re2 agent online: %s (%s)", data.Name, deviceID)

		case re2.TypePairOffer:
			if deviceID == "" {
				_ = writeRE2Err(c, f.RouteID, "not_registered", "register first")
				continue
			}
			var data re2.PairOfferPayload
			if err := json.Unmarshal(f.Payload, &data); err != nil {
				_ = writeRE2Err(c, deviceID, "bad_data", err.Error())
				continue
			}
			relay := publicRelayRE2(hub, r)
			ttl := time.Until(time.Unix(data.ExpiresAt, 0))
			if ttl <= 0 {
				ttl = 10 * time.Minute
			}
			hub.auth.PutPairing(deviceID, data.Name, relay, data.PairingToken, ttl)
			log.Printf("re2 pair_offer device=%s", deviceID)

		case re2.TypeNoise, re2.TypeTunnel:
			if deviceID == "" {
				continue
			}
			route := f.RouteID
			if route == "" {
				route = deviceID
			}
			logOpaque(f.Type, route, len(f.Payload))
			cli := hub.re2ClientOf(route)
			if cli == nil {
				continue
			}
			_ = cli.WriteFrame(re2.Frame{Type: f.Type, RouteID: route, Payload: f.Payload})

		case re2.TypePing:
			_ = c.WriteFrame(re2.Frame{Type: re2.TypePong, RouteID: f.RouteID, Payload: f.Payload})

		case re2.TypePong:
			// ignore

		default:
			log.Printf("re2 agent unexpected type=%s device=%s", re2.FrameTypeName(f.Type), deviceID)
		}
	}
}

func serveRE2Client(hub *Hub, c *re2.Conn, r *http.Request) {
	deviceID := ""
	acquired := false
	defer func() {
		if deviceID != "" {
			peer := hub.clearRE2Client(deviceID, c)
			notifyPeerGone(peer, deviceID)
		}
		if acquired && hub.limiter != nil {
			hub.limiter.Release()
		}
	}()

	for {
		f, err := c.ReadFrame()
		if err != nil {
			return
		}
		switch f.Type {
		case re2.TypePairRedeem:
			var data re2.PairRedeemPayload
			if err := json.Unmarshal(f.Payload, &data); err != nil {
				_ = writeRE2Err(c, f.RouteID, "bad_data", err.Error())
				continue
			}
			ack, ok := hub.auth.RedeemPairing(data.DeviceID, data.PairingToken)
			if !ok {
				_ = writeRE2Err(c, data.DeviceID, "pair_failed", "invalid or expired pairing token")
				continue
			}
			_ = c.WriteFrame(re2.Frame{
				Type:    re2.TypePairAck,
				RouteID: ack.DeviceID,
				Payload: re2.MustJSON(re2.PairAckPayload{
					DeviceID:      ack.DeviceID,
					SessionTicket: ack.SessionToken,
					Name:          ack.Name,
				}),
			})
			log.Printf("re2 pair_redeem ok device=%s", ack.DeviceID)

		case re2.TypeBind:
			var data re2.BindPayload
			if err := json.Unmarshal(f.Payload, &data); err != nil {
				_ = writeRE2Err(c, f.RouteID, "bad_data", err.Error())
				continue
			}
			if !hub.auth.ValidateSession(data.DeviceID, data.SessionTicket) {
				_ = writeRE2Err(c, data.DeviceID, "auth_failed", "invalid session ticket")
				return
			}
			if hub.re2AgentOf(data.DeviceID) == nil {
				_ = writeRE2Err(c, data.DeviceID, "offline", "device agent offline")
				continue
			}
			if hub.limiter != nil && !hub.limiter.TryAcquire() {
				_ = writeRE2Err(c, data.DeviceID, "relay_full",
					fmt.Sprintf("relay at capacity (%d sessions); try another node", hub.limiter.Max()))
				continue
			}
			acquired = true
			deviceID = data.DeviceID
			hub.setRE2Client(deviceID, c)
			_ = c.WriteFrame(re2.Frame{
				Type:    re2.TypeBindOK,
				RouteID: deviceID,
				Payload: re2.MustJSON(re2.BindOKPayload{OK: true, UDP: hub.publicUDP}),
			})
			log.Printf("re2 bind ok device=%s active=%d/%d", deviceID, hub.limiter.Active(), hub.limiter.Max())

		case re2.TypeNoise, re2.TypeTunnel:
			if deviceID == "" {
				continue
			}
			route := f.RouteID
			if route == "" {
				route = deviceID
			}
			logOpaque(f.Type, route, len(f.Payload))
			agent := hub.re2AgentOf(route)
			if agent == nil {
				_ = writeRE2Err(c, route, "offline", "device agent offline")
				continue
			}
			_ = agent.WriteFrame(re2.Frame{Type: f.Type, RouteID: route, Payload: f.Payload})

		case re2.TypePing:
			_ = c.WriteFrame(re2.Frame{Type: re2.TypePong, RouteID: f.RouteID, Payload: f.Payload})

		case re2.TypePong:
			// ignore

		default:
			log.Printf("re2 client unexpected type=%s device=%s", re2.FrameTypeName(f.Type), deviceID)
		}
	}
}

func publicRelayRE2(hub *Hub, r *http.Request) string {
	base := publicRelay(hub, r)
	return re2.EnsurePath(base)
}
