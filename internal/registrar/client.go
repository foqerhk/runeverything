package registrar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ClaimRequest is sent by a volunteer relay to obtain a public hostname.
type ClaimRequest struct {
	IP     string `json:"ip"`
	NodeID string `json:"node_id,omitempty"`
	Port   int    `json:"port,omitempty"` // informational; public wss uses 443
}

// ClaimResponse is returned after DNS is pointed at the volunteer.
type ClaimResponse struct {
	Hostname string `json:"hostname"`
	WSSURL   string `json:"wss_url"`
	IP       string `json:"ip"`
}

// ReportRequest is sent by home agents/clients after a real failed use attempt.
// No join token — the official registrar dedupes and scores server-side.
type ReportRequest struct {
	URL        string `json:"url"` // wss://host/re2 or https://host
	Reason     string `json:"reason"`
	Detail     string `json:"detail,omitempty"`
	ReporterID string `json:"reporter_id,omitempty"`
}

// ReportResponse is returned by POST /v1/report.
type ReportResponse struct {
	Accepted bool    `json:"accepted"`
	Hostname string  `json:"hostname,omitempty"`
	Score    float64 `json:"score,omitempty"`
	Revoked  bool    `json:"revoked,omitempty"`
	Message  string  `json:"message,omitempty"`
}

// EnrollRequest asks for a per-node join token (when volunteer enables sharing).
type EnrollRequest struct {
	NodeID string `json:"node_id"`
}

// EnrollResponse carries the node-scoped join token to store locally.
type EnrollResponse struct {
	NodeID    string `json:"node_id"`
	JoinToken string `json:"join_token"`
	Rotated   bool   `json:"rotated,omitempty"`
	Message   string `json:"message,omitempty"`
}

// Client talks to the official registrar (closed-source ops service).
type Client struct {
	BaseURL   string
	JoinToken string
	HTTP      *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Enroll obtains (or rotates) a per-node join token. No shared secret required.
func (c *Client) Enroll(ctx context.Context, req EnrollRequest) (*EnrollResponse, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("registrar URL empty")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/enroll", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("enroll %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	var out EnrollResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.JoinToken == "" || out.NodeID == "" {
		return nil, fmt.Errorf("invalid enroll response")
	}
	return &out, nil
}

// Claim asks the registrar to publish DNS for this node's public IP.
func (c *Client) Claim(ctx context.Context, req ClaimRequest) (*ClaimResponse, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("registrar URL empty")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/claim", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if t := strings.TrimSpace(c.JoinToken); t != "" {
		httpReq.Header.Set("Authorization", "Bearer "+t)
	}
	res, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("claim %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	var out ClaimResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Hostname == "" || out.WSSURL == "" {
		return nil, fmt.Errorf("invalid claim response")
	}
	return &out, nil
}

// Report notifies the registrar that a hostname failed real use / probe.
// No join token required — home users report freely (deduped server-side).
func (c *Client) Report(ctx context.Context, req ReportRequest) (*ReportResponse, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("registrar URL empty")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/report", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("report %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	var out ReportResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidPublicIP reports whether ip is a non-private unicast address.
func ValidPublicIP(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	return true
}
