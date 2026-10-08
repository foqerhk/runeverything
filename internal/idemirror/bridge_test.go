package idemirror

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"
)

func TestBuildVSIX(t *testing.T) {
	raw, err := buildVSIX()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"[Content_Types].xml": false, "extension.vsixmanifest": false, "extension/package.json": false, "extension/extension.js": false}
	for _, f := range zr.File {
		want[f.Name] = true
	}
	for name, ok := range want {
		if !ok {
			t.Fatalf("missing %s", name)
		}
	}
	if out := os.Getenv("KOKO_VSIX_OUT"); out != "" {
		if err := os.WriteFile(out, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPickBridgePrefersWorkspace(t *testing.T) {
	regs := []bridgeReg{
		{Port: 1, Folders: []string{"/a"}, Focused: true},
		{Port: 2, Folders: []string{"/work/koko"}},
	}
	if r, _ := pickBridge(regs, "/work/koko/ios"); r.Port != 2 {
		t.Fatalf("picked %d", r.Port)
	}
	if r, _ := pickBridge(regs, "/elsewhere"); r.Port != 1 {
		t.Fatalf("fallback picked %d", r.Port)
	}
}
