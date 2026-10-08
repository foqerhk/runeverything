//go:build !darwin

package idemirror

import "errors"

type otherDesk struct{}

func newDesk() desk { return otherDesk{} }

func (otherDesk) Trusted() bool                { return false }
func (otherDesk) Raise(int, string) error      { return errors.New("not supported on this OS yet") }
func (otherDesk) Frontmost() int               { return 0 }
func (otherDesk) PasteAndSubmit(string)        {}
func (otherDesk) Paste(string)                 {}
func (otherDesk) PressButton(int, string) bool { return false }
