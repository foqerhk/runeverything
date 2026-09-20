//go:build !windows

package winutil

import "errors"

var ErrWindowsOnly = errors.New("windows only")

func InstallDir() string                            { return "" }
func AgentPath() string                             { return "" }
func EnsureInstallDir() error                       { return ErrWindowsOnly }
func AddUserPath(string) error                      { return ErrWindowsOnly }
func CreateStartMenuShortcut(string) error          { return ErrWindowsOnly }
func RegisterLogonTask(string) error                { return ErrWindowsOnly }
func UnregisterLogonTask() error                    { return ErrWindowsOnly }
func LogonTaskExists() bool                         { return false }
func OpenFile(string) error                         { return ErrWindowsOnly }
func OpenFolder(string) error                       { return ErrWindowsOnly }
func SetClipboardText(string) error                 { return ErrWindowsOnly }
func NotifyBalloon(string, string)                  {}
func FreeConsole()                                  {}
func StartMenuShortcutPath() string                 { return "" }
const TaskName = "RunEverythingAgent"
