//go:build !darwin && !linux && !windows

package main

func acquireAgentLock() (func(), error) {
	return func() {}, nil
}
