//go:build unix

package utils

import (
	"os/signal"
	"syscall"
)

func init() {
	registerSIGHUP = func(s *ShutdownManager) {
		signal.Notify(s.signals, syscall.SIGHUP)
	}
}
