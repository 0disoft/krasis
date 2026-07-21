//go:build windows

package main

import (
	"fmt"
	"os"

	"github.com/0disoft/krasis/internal/launcherui"
)

func main() {
	if err := launcherui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
