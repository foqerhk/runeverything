//go:build darwin

package desktop

func ListMonitors() ([]Monitor, error) {
	return listMonitorsCG()
}
