package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/i18n"
)

const defaultUpdateRepo = "foqerhk/runeverything"

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func updateRepo() string {
	if v := strings.TrimSpace(os.Getenv("RE_UPDATE_REPO")); v != "" {
		return v
	}
	return defaultUpdateRepo
}

func checkForUpdate() (msg string, downloadURL string, newer bool) {
	repo := updateRepo()
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	client := &http.Client{Timeout: 12 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return i18n.T("ui.update_fail", err.Error()), "", false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "runeverything/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return i18n.T("ui.update_fail", err.Error()), "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return i18n.T("ui.update_fail", fmt.Sprintf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))), "", false
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return i18n.T("ui.update_fail", err.Error()), "", false
	}
	latest := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	cur := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if latest == "" {
		return i18n.T("ui.update_none"), "", false
	}
	if !isVersionNewer(latest, cur) {
		return i18n.T("ui.update_latest", cur), "", false
	}
	dl := pickReleaseAsset(rel)
	if dl == "" {
		dl = rel.HTMLURL
	}
	return i18n.T("ui.update_available", latest, cur), dl, true
}

func pickReleaseAsset(rel githubRelease) string {
	osName, arch := runtime.GOOS, runtime.GOARCH
	want := []string{}
	switch {
	case osName == "darwin" && arch == "arm64":
		want = []string{"darwin_arm64", "darwin-arm64", "macos_arm64"}
	case osName == "darwin":
		want = []string{"darwin_amd64", "darwin-amd64", "macos_amd64"}
	case osName == "windows" && arch == "arm64":
		want = []string{"windows_arm64", "windows-arm64"}
	case osName == "windows":
		want = []string{"windows_amd64", "windows-amd64"}
	case osName == "linux" && arch == "arm64":
		want = []string{"linux_arm64", "linux-arm64"}
	default:
		want = []string{"linux_amd64", "linux-amd64"}
	}
	for _, a := range rel.Assets {
		name := strings.ToLower(a.Name)
		for _, w := range want {
			if strings.Contains(name, w) {
				return a.BrowserDownloadURL
			}
		}
	}
	if rel.HTMLURL != "" {
		return rel.HTMLURL
	}
	return ""
}

// isVersionNewer reports whether remote looks newer than local (best-effort semver-ish).
func isVersionNewer(remote, local string) bool {
	remote = strings.TrimSuffix(remote, "-dev")
	local = strings.TrimSuffix(local, "-dev")
	if remote == local {
		return false
	}
	rp := splitVer(remote)
	lp := splitVer(local)
	for i := 0; i < 3; i++ {
		r, l := 0, 0
		if i < len(rp) {
			r = rp[i]
		}
		if i < len(lp) {
			l = lp[i]
		}
		if r > l {
			return true
		}
		if r < l {
			return false
		}
	}
	// Equal numeric prefix but different strings (e.g. tags) — treat remote tag inequality as newer.
	return remote != local
}

func splitVer(s string) []int {
	s = strings.SplitN(s, "-", 2)[0]
	parts := strings.Split(s, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n := 0
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				break
			}
			n = n*10 + int(ch-'0')
		}
		out = append(out, n)
	}
	return out
}

func openUpdateURL(url string) {
	if url == "" {
		return
	}
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Start()
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}
