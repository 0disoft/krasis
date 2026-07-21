//go:build !windows

package launcher

import "os/exec"

func configureProcess(*exec.Cmd) {}
