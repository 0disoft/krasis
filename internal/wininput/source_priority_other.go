//go:build !windows

package wininput

func RunSourceInputTask(work func()) { work() }
