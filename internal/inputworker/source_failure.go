package inputworker

// Fixed protocol operations only. This never includes a key/button value,
// cursor coordinate, peer identity, native error text or caller-provided string.
func sourceCommandFailureCode(op byte) string {
	name := "unknown"
	switch op {
	case sourceStart:
		name = "start"
	case sourceAdmit:
		name = "admit"
	case sourceActivate:
		name = "activate"
	case sourceDeactivate:
		name = "deactivate"
	case sourceHeartbeat:
		name = "heartbeat"
	case sourceStop:
		name = "stop"
	case sourceCursorPrepare:
		name = "cursor_prepare"
	case sourceCursorShow:
		name = "cursor_show"
	case sourceCursorRestore:
		name = "cursor_restore"
	}
	return "installed_source_command_failed_" + name
}
