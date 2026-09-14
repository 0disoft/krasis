//go:build windows

package main

import (
	"github.com/0disoft/krasis/internal/launcherui"
	"os"
)

func main() {
	if launcherui.RunProductControl() != nil {
		os.Exit(1)
	}
}
