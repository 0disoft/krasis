//go:build !windows

package wininput

import "context"

func CredentialBrokerForeground() bool { return false }

func RunCredentialUIProbe(context.Context) CredentialUIProbeResult {
	return CredentialUIProbeNotElevated
}
