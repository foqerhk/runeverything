package registrar

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/foqerhk/runeverything/internal/identity"
)

type savedJoin struct {
	NodeID    string `json:"node_id"`
	JoinToken string `json:"join_token"`
	Registrar string `json:"registrar_url,omitempty"`
}

func joinTokenPath() (string, error) {
	dir, err := identity.EnsureHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "node-join-token.json"), nil
}

// LoadSavedJoinToken reads a previously enrolled token from disk.
func LoadSavedJoinToken() (nodeID, token string, err error) {
	path, err := joinTokenPath()
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var s savedJoin
	if err := json.Unmarshal(data, &s); err != nil {
		return "", "", err
	}
	if s.JoinToken == "" {
		return "", "", fmt.Errorf("empty join token file")
	}
	return s.NodeID, s.JoinToken, nil
}

// SaveJoinToken persists the enrolled token for this volunteer node.
func SaveJoinToken(nodeID, token, registrarURL string) error {
	path, err := joinTokenPath()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(savedJoin{
		NodeID:    nodeID,
		JoinToken: token,
		Registrar: registrarURL,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// EnsureJoinToken returns env token, or saved token, or enrolls a new one when sharing.
func EnsureJoinToken(ctx context.Context, registrarURL, nodeID string) (token string, err error) {
	if t := strings.TrimSpace(os.Getenv("RE_JOIN_TOKEN")); t != "" {
		return t, nil
	}
	if _, t, e := LoadSavedJoinToken(); e == nil && t != "" {
		return t, nil
	}
	cli := &Client{BaseURL: registrarURL}
	resp, err := cli.Enroll(ctx, EnrollRequest{NodeID: nodeID})
	if err != nil {
		return "", err
	}
	if err := SaveJoinToken(resp.NodeID, resp.JoinToken, registrarURL); err != nil {
		log.Printf("warning: could not save join token: %v", err)
	} else {
		log.Printf("registrar: enrolled node=%s (token saved under ~/.runeverything/)", resp.NodeID)
	}
	return resp.JoinToken, nil
}
