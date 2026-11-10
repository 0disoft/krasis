//go:build !linux

package main

import "context"

func startDesktopServices(context.Context, string) func() { return func() {} }
