//go:build windows && desktop

package workspacehost

import "time"

func releaseCheckDelay(failures int) time.Duration {
	switch failures {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 15 * time.Minute
	default:
		return 6 * time.Hour
	}
}
