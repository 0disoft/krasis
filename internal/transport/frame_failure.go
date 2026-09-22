package transport

import "errors"

var (
	ErrFrameHeaderTruncated  = errors.New("frame header truncated")
	ErrFramePayloadTruncated = errors.New("frame payload truncated")
	ErrFrameMagic            = errors.New("frame magic mismatch")
	ErrFrameVersion          = errors.New("frame version unsupported")
	ErrFrameKind             = errors.New("frame kind unsupported")
	ErrFrameFlags            = errors.New("frame flags unsupported")
)

// FrameFailureCode is a closed vocabulary. It never returns payload or peer data.
func FrameFailureCode(err error) string {
	for _, entry := range []struct {
		cause error
		code  string
	}{
		{ErrFrameHeaderTruncated, "header_truncated"}, {ErrFramePayloadTruncated, "payload_truncated"},
		{ErrFrameMagic, "magic_invalid"}, {ErrFrameVersion, "version_unsupported"}, {ErrFrameKind, "kind_unsupported"}, {ErrFrameFlags, "flags_unsupported"},
		{ErrPayloadTooLarge, "payload_oversized"},
		{ErrInvalidControlPayload, "control_invalid"}, {ErrUnsupportedControlPayload, "control_unsupported"},
		{ErrInvalidInputPayload, "input_invalid"}, {ErrUnsupportedInputPayload, "input_unsupported"},
		{ErrInvalidHeartbeatPayload, "heartbeat_invalid"}, {ErrUnsupportedHeartbeatPayload, "heartbeat_unsupported"},
	} {
		if errors.Is(err, entry.cause) {
			return entry.code
		}
	}
	return ""
}
