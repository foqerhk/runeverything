//go:build !darwin && !linux && !windows

package desktop

type noopInjector struct{}

func NewInjector() (Injector, error) {
	return &noopInjector{}, nil
}

func (n *noopInjector) Move(x, y float64) error                    { return nil }
func (n *noopInjector) MoveRelative(dx, dy float64) error           { return nil }
func (n *noopInjector) Button(buttons int, down bool) error         { return nil }
func (n *noopInjector) Wheel(delta int) error                       { return nil }
func (n *noopInjector) WheelH(delta int) error                      { return nil }
func (n *noopInjector) Key(keyCode int, text string, down bool, modifiers int) error {
	return nil
}
func (n *noopInjector) SetRelativeMouse(on bool) {}
func (n *noopInjector) Close() error             { return nil }
