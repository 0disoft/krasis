package wininput

import "errors"

var ErrProtectedInput = errors.New("protected Windows input UI is active")

// ProtectedInputReason contains only bounded OS observations, never window
// content or process paths. Unknown identity retains the conservative guard.
type ProtectedInputReason uint8

const (
	ProtectedInputClear ProtectedInputReason = iota
	ProtectedInputDesktopUnavailable
	ProtectedInputNonDefaultDesktop
	ProtectedInputCredentialBroker
	ProtectedInputIdentityUnknown
)
