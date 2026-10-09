//go:build windows && !desktop

package main

import (
	"github.com/0disoft/krasis/internal/launcherui"
	"os"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == launcherui.ClipboardPackArgument {
		os.Exit(launcherui.RunClipboardFilePacker(os.Stdin, os.Stdout))
	}
	if len(os.Args) == 5 && os.Args[1] == "--updated-peer" && os.Args[3] == "--expected-build" && len(os.Args[4]) == 40 {
		if launcherui.RunUpdatedProductControl(os.Args[2], os.Args[4]) != nil {
			os.Exit(1)
		}
		return
	}
	if launcherui.RunProductControl() != nil {
		os.Exit(1)
	}
}
