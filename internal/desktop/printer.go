package desktop

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// PrinterInfoLocal is a local printer.
type PrinterInfoLocal struct {
	ID      string
	Name    string
	Default bool
}

func ListPrinters() ([]PrinterInfoLocal, error) {
	switch runtime.GOOS {
	case "darwin", "linux":
		return listPrintersCUPS()
	case "windows":
		return listPrintersWindows()
	default:
		return nil, fmt.Errorf("printer: unsupported")
	}
}

func listPrintersCUPS() ([]PrinterInfoLocal, error) {
	out, err := exec.Command("lpstat", "-a").Output()
	if err != nil {
		// try lpstat -p
		out, err = exec.Command("lpstat", "-p").Output()
		if err != nil {
			return nil, err
		}
	}
	defName := ""
	if d, err := exec.Command("lpstat", "-d").Output(); err == nil {
		// system default destination: Foo
		s := string(d)
		if i := strings.LastIndex(s, ":"); i >= 0 {
			defName = strings.TrimSpace(s[i+1:])
		}
	}
	var printers []PrinterInfoLocal
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if name == "printer" && len(fields) > 1 {
			name = fields[1]
		}
		printers = append(printers, PrinterInfoLocal{
			ID: name, Name: name, Default: name == defName,
		})
	}
	return printers, nil
}

func listPrintersWindows() ([]PrinterInfoLocal, error) {
	ps := `Get-Printer | ForEach-Object { $_.Name + "|" + $_.Default }`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err != nil {
		return nil, err
	}
	var printers []PrinterInfoLocal
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		name := parts[0]
		def := len(parts) > 1 && strings.EqualFold(parts[1], "True")
		printers = append(printers, PrinterInfoLocal{ID: name, Name: name, Default: def})
	}
	return printers, nil
}

// SubmitPrintJob writes data to a temp file and sends to the named printer.
func SubmitPrintJob(printerID, jobName, mime string, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty job")
	}
	dir, err := XferDir()
	if err != nil {
		return err
	}
	ext := ".bin"
	switch mime {
	case "application/pdf":
		ext = ".pdf"
	case "image/png":
		ext = ".png"
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "text/plain":
		ext = ".txt"
	}
	if jobName == "" {
		jobName = "re-print"
	}
	path := filepath.Join(dir, sanitizePrintName(jobName)+ext)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	defer os.Remove(path)

	switch runtime.GOOS {
	case "darwin", "linux":
		args := []string{path}
		if printerID != "" {
			args = []string{"-d", printerID, path}
		}
		return exec.Command("lp", args...).Run()
	case "windows":
		// Start-Process -Verb Print
		ps := fmt.Sprintf(`Start-Process -FilePath '%s' -Verb Print -WindowStyle Hidden`, strings.ReplaceAll(path, `'`, `''`))
		if printerID != "" {
			ps = fmt.Sprintf(`$p='%s'; Start-Process -FilePath '%s' -Verb PrintTo -ArgumentList $p -WindowStyle Hidden`,
				strings.ReplaceAll(printerID, `'`, `''`), strings.ReplaceAll(path, `'`, `''`))
		}
		return exec.Command("powershell", "-NoProfile", "-Command", ps).Run()
	default:
		return fmt.Errorf("print unsupported")
	}
}

func sanitizePrintName(s string) string {
	s = filepath.Base(s)
	s = strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, s)
	if s == "" {
		return "job"
	}
	return s
}

func DecodeB64(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
