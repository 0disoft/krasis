//go:build !windows && !linux

package launcher

import "os/exec"

func configureProcess(*exec.Cmd) {}
