//go:build windows && !embedagent

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// When not built with -tags embedagent, download the agent from GitHub Releases.
func embeddedAgent() ([]byte, error) {
	arch := "amd64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	ver := strings.TrimSpace(version)
	var url string
	if ver == "" || ver == "dev" || ver == "latest" {
		url = fmt.Sprintf("https://github.com/foqerhk/runeverything/releases/latest/download/runeverything_windows_%s.exe", arch)
	} else {
		v := ver
		if !strings.HasPrefix(v, "v") {
			v = "v" + v
		}
		url = fmt.Sprintf("https://github.com/foqerhk/runeverything/releases/download/%s/runeverything_windows_%s.exe", v, arch)
	}
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download agent: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download agent: HTTP %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// Avoid unused when building download-only setup on machines with local copy path.
var _ = os.ErrNotExist
