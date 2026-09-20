//go:build !darwin && !linux && !windows

package desktop

func ListMonitors() ([]Monitor, error) {
	return []Monitor{{ID: 0, Name: "Virtual", Width: 1280, Height: 720, Primary: true}}, nil
}
