//go:build windows

package inputworker

// Called only after the serial reader verified the request and its operation
// finished. Repeating WTS/SCM here can obscure a completed operation behind a
// new query's delay/failure. This path carries only the exact result; it cannot
// send input, change an epoch or certify native retirement. Pinned owner
// presence, wire serialization and the ordinary write deadline still apply.
func (w *sourceWriter) sendCompletedReply(request, response sourcePacket) error {
	if request.kind != sourceCommand || !request.valid() || request.binding != w.binding || request.op == sourceStop ||
		response.kind != sourceReply || response.op != request.op || response.reply != request.sequence ||
		response.sequence != 0 || response.binding != (Binding{}) {
		return ErrProtocol
	}
	validated := response
	validated.sequence, validated.binding = 1, w.binding
	if !validated.valid() {
		return ErrProtocol
	}
	return w.writePacket(response, nil, false, true)
}
