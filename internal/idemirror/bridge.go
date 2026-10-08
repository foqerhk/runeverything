package idemirror

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed cursorext/package.json cursorext/extension.js
var cursorExt embed.FS

const (
	bridgeExtID      = "runeverything.koko-ide-bridge"
	bridgeExtVersion = "0.2.0"
)

// bridgeReg is written by the IDE extension, one file per IDE window.
type bridgeReg struct {
	Version   string   `json:"version"`
	App       string   `json:"app"`
	Scheme    string   `json:"scheme"`
	PID       int      `json:"pid"`
	PPID      int      `json:"ppid"`
	Port      int      `json:"port"`
	Token     string   `json:"token"`
	Folders   []string `json:"folders"`
	Focused   bool     `json:"focused"`
	UpdatedMs int64    `json:"updatedMs"`
}

func bridgeDir(home string) string {
	return filepath.Join(home, ".runeverything", "ide-bridge")
}

// bridges returns live Cursor windows; stale files of exited extension hosts are removed.
func bridges(home string) []bridgeReg {
	dir := bridgeDir(home)
	entries, _ := os.ReadDir(dir)
	var out []bridgeReg
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var r bridgeReg
		if json.Unmarshal(raw, &r) != nil || r.Port == 0 || r.Token == "" {
			continue
		}
		if r.Scheme != "" && r.Scheme != "cursor" {
			continue
		}
		if r.PID > 0 && !processAlive(r.PID) {
			_ = os.Remove(path)
			continue
		}
		out = append(out, r)
	}
	return out
}

// pickBridge prefers the window that has the chat's workspace open.
func pickBridge(regs []bridgeReg, cwd string) (bridgeReg, bool) {
	if len(regs) == 0 {
		return bridgeReg{}, false
	}
	if cwd != "" {
		for _, r := range regs {
			for _, f := range r.Folders {
				if f == cwd || strings.HasPrefix(cwd, f+string(os.PathSeparator)) {
					return r, true
				}
			}
		}
	}
	for _, r := range regs {
		if r.Focused {
			return r, true
		}
	}
	return regs[0], true
}

// bridgeSupports reports whether the window runs a bridge with the /command endpoint
// (0.2.0+; 0.1.0 did not publish a version). Older windows need Reload Window.
func bridgeSupports(r bridgeReg) bool { return r.Version != "" }

// bridgeForFolder returns the window that has exactly this folder open.
func bridgeForFolder(regs []bridgeReg, folder string) (bridgeReg, bool) {
	want := realPath(folder)
	for _, r := range regs {
		for _, f := range r.Folders {
			if f == folder || realPath(f) == want {
				return r, true
			}
		}
	}
	return bridgeReg{}, false
}

func realPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

type bridgeReply struct {
	OK      bool   `json:"ok"`
	Focused bool   `json:"focused"`
	Error   string `json:"error"`
}

func callBridge(r bridgeReg, endpoint string, body any) (bridgeReply, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/%s", r.Port, endpoint), bytes.NewReader(raw))
	if err != nil {
		return bridgeReply{}, err
	}
	req.Header.Set("x-koko-token", r.Token)
	req.Header.Set("content-type", "application/json")
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return bridgeReply{}, err
	}
	defer resp.Body.Close()
	var rep bridgeReply
	_ = json.NewDecoder(resp.Body).Decode(&rep)
	if !rep.OK {
		if rep.Error == "" {
			rep.Error = resp.Status
		}
		return rep, errors.New(rep.Error)
	}
	return rep, nil
}

func cursorExtensionsDir(home string) string {
	return filepath.Join(home, ".cursor", "extensions")
}

func bridgeInstalled(home string) bool {
	_, err := os.Stat(filepath.Join(cursorExtensionsDir(home), bridgeExtID+"-"+bridgeExtVersion, "package.json"))
	return err == nil
}

func cursorCLI() string {
	candidates := []string{"cursor"}
	if runtime.GOOS == "darwin" {
		candidates = append([]string{"/Applications/Cursor.app/Contents/Resources/app/bin/cursor"}, candidates...)
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

// installBridge installs the embedded extension into Cursor via its CLI.
func installBridge(home string) error {
	cli := cursorCLI()
	if cli == "" {
		return errors.New("cursor CLI not found")
	}
	vsix, err := buildVSIX()
	if err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("koko-ide-bridge-%s.vsix", bridgeExtVersion))
	if err := os.WriteFile(tmp, vsix, 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	out, err := exec.Command(cli, "--install-extension", tmp, "--force").CombinedOutput()
	if err != nil {
		return fmt.Errorf("install extension: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func buildVSIX() ([]byte, error) {
	pkg, err := cursorExt.ReadFile("cursorext/package.json")
	if err != nil {
		return nil, err
	}
	js, err := cursorExt.ReadFile("cursorext/extension.js")
	if err != nil {
		return nil, err
	}
	manifest := `<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011" xmlns:d="http://schemas.microsoft.com/developer/vsx-schema-design/2011">
  <Metadata>
    <Identity Language="en-US" Id="koko-ide-bridge" Version="` + bridgeExtVersion + `" Publisher="runeverything" />
    <DisplayName>KoKo IDE Bridge</DisplayName>
    <Description xml:space="preserve">Lets the RunEverything Agent deliver KoKo phone input into IDE agent chats.</Description>
    <Properties>
      <Property Id="Microsoft.VisualStudio.Code.Engine" Value="^1.80.0" />
      <Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="ui" />
    </Properties>
  </Metadata>
  <Installation>
    <InstallationTarget Id="Microsoft.VisualStudio.Code" />
  </Installation>
  <Dependencies />
  <Assets>
    <Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true" />
  </Assets>
</PackageManifest>
`
	contentTypes := `<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension=".json" ContentType="application/json" /><Default Extension=".js" ContentType="application/javascript" /><Default Extension=".vsixmanifest" ContentType="text/xml" /></Types>
`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct {
		name string
		data []byte
	}{
		{"[Content_Types].xml", []byte(contentTypes)},
		{"extension.vsixmanifest", []byte(manifest)},
		{"extension/package.json", pkg},
		{"extension/extension.js", js},
	} {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
