//go:build !windows

package crashguard

import "os/exec"

func configureProcess(_ *exec.Cmd) {}
