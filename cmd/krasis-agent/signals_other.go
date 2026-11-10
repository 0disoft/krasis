//go:build !linux

package main

import "os"

func residentSignals() []os.Signal { return []os.Signal{os.Interrupt} }
