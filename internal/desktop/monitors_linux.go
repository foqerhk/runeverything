//go:build linux

package desktop

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var xrandrLine = regexp.MustCompile(`^(\S+)\s+(connected(?:\s+primary)?)\s+(\d+)x(\d+)\+(\d+)\+(\d+)`)

func ListMonitors() ([]Monitor, error) {
	if mons, err := listMonitorsXrandr(); err == nil && len(mons) > 0 {
		return mons, nil
	}
	if os.Getenv("DISPLAY") == "" {
		_ = os.Setenv("DISPLAY", ":10")
	}
	c, err := newX11Capturer()
	if err != nil {
		return []Monitor{{ID: 0, Name: "DISPLAY", Width: 1920, Height: 1080, Primary: true}}, nil
	}
	w, h := c.Size()
	return []Monitor{{ID: 0, Name: os.Getenv("DISPLAY"), Width: w, Height: h, Primary: true}}, nil
}

func listMonitorsXrandr() ([]Monitor, error) {
	cmd := exec.Command("xrandr", "--query")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var mons []Monitor
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		m := xrandrLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		w, _ := strconv.Atoi(m[3])
		h, _ := strconv.Atoi(m[4])
		x, _ := strconv.Atoi(m[5])
		y, _ := strconv.Atoi(m[6])
		primary := strings.Contains(m[2], "primary")
		mons = append(mons, Monitor{
			ID: len(mons), Name: m[1],
			Width: w, Height: h, X: x, Y: y, Primary: primary,
		})
	}
	if len(mons) == 0 {
		return nil, os.ErrNotExist
	}
	// Ensure one primary.
	hasPri := false
	for _, m := range mons {
		if m.Primary {
			hasPri = true
			break
		}
	}
	if !hasPri {
		mons[0].Primary = true
	}
	return mons, nil
}
