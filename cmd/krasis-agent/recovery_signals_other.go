//go:build !linux

package main

func preserveRecoveryUntilParentEOF() func() { return func() {} }
