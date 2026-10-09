package main

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/foqerhk/runeverything/internal/agentchat"
	"github.com/foqerhk/runeverything/internal/audit"
	"github.com/foqerhk/runeverything/internal/re2"
	"github.com/foqerhk/runeverything/internal/reudp"
)

// handleAgentChat is a data-only path: no desktop capture, inject, or PTY.
// send replies on the channel the request came from.
func (a *Agent) handleAgentChat(mt byte, body []byte, send func(byte, []byte) error) error {
	switch mt {
	case re2.MsgAgentChatList:
		var p re2.AgentChatListPayload
		if len(body) > 0 {
			_ = json.Unmarshal(body, &p)
		}
		sessions, err := agentchat.List(p.ProjectPath)
		out := re2.AgentChatListPayload{Action: "list"}
		if err != nil {
			out.Error = err.Error()
			return send(re2.MsgAgentChatList, re2.MustJSON(out))
		}
		filter := strings.ToLower(strings.TrimSpace(p.Kind))
		for _, s := range sessions {
			if filter != "" && s.Kind != filter {
				continue
			}
			out.Sessions = append(out.Sessions, re2.AgentChatInfo{
				Kind: s.Kind, ID: s.ID, Title: s.Title, Cwd: s.Cwd,
				CreatedAtMs: s.CreatedAtMs, UpdatedAtMs: s.UpdatedAtMs,
				ScreenName: s.ScreenName, ScreenAlive: s.ScreenAlive,
				Source: s.Source, Client: s.Client,
			})
		}
		offset := p.Offset
		if offset < 0 {
			offset = 0
		}
		if offset > len(out.Sessions) {
			offset = len(out.Sessions)
		}
		all := out.Sessions
		out.Sessions = append([]re2.AgentChatInfo(nil), all[offset:]...)
		out.Offset = offset
		// REUDP MaxPayload≈1200; page the complete inventory instead of silently
		// dropping every older provider after the newest Cursor rows.
		trimmed := trimAgentChatList(out, reudp.MaxPayload-64)
		trimmed.NextOffset = offset + len(trimmed.Sessions)
		trimmed.HasMore = trimmed.NextOffset < len(all)
		// Metadata changes JSON size; leave one more row out if it crosses the cap.
		for len(trimmed.Sessions) > 0 && len(re2.MustJSON(trimmed)) > reudp.MaxPayload-64 {
			trimmed.Sessions = trimmed.Sessions[:len(trimmed.Sessions)-1]
			trimmed.NextOffset = offset + len(trimmed.Sessions)
			trimmed.HasMore = trimmed.NextOffset < len(all)
		}
		if trimmed.HasMore {
			log.Printf("agent chat: page offset=%d count=%d total=%d", offset, len(trimmed.Sessions), len(all))
		}
		audit.Log("agent_chat_list", strconv.Itoa(len(trimmed.Sessions)))
		return send(re2.MsgAgentChatList, re2.MustJSON(trimmed))

	case re2.MsgAgentChatDetail:
		var p re2.AgentChatDetailPayload
		if err := json.Unmarshal(body, &p); err != nil {
			return err
		}
		// Hook for future transcript / detail fetch — list-only for now.
		return send(re2.MsgAgentChatDetail, re2.MustJSON(re2.AgentChatDetailPayload{
			Action: "get",
			Kind:   p.Kind,
			ID:     p.ID,
			OK:     false,
			Error:  "agent_chat_detail_not_implemented",
		}))
	}
	return nil
}

// trimAgentChatList drops oldest-looking entries until JSON fits maxBody bytes
// (caller should leave room for EncodeInner + Noise tag under REUDP MaxPayload).
func trimAgentChatList(out re2.AgentChatListPayload, maxBody int) re2.AgentChatListPayload {
	if maxBody < 64 {
		maxBody = 64
	}
	for len(out.Sessions) > 0 {
		raw := re2.MustJSON(out)
		if len(raw) <= maxBody {
			return out
		}
		// Drop from the end — list order is newest-first from agentchat.List.
		out.Sessions = out.Sessions[:len(out.Sessions)-1]
	}
	return out
}
