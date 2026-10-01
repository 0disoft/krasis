//go:build !windows

package residentagent

import "time"

func installedProductInput(time.Time, func(error)) *privilegedLeaseInput { return nil }
