//go:build linux

package main

import (
	"os"
	"syscall"
)

func residentSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
