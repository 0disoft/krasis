package transport

import (
	"crypto/sha256"
	"encoding/hex"
)

const handoffDiagnosticLabel = "krasis/handoff-diagnostic-correlation/v1"

// DiagnosticExchangeTag links one request/grant on the two authenticated peers.
// A dedicated TLS exporter context keeps tags connection-scoped and opaque even
// for guessable identifiers. Only a digest is returned; exporter bytes, input
// identifiers and timestamps never leave this method. No I/O or handshake runs.
func (connection *PeerConnection) DiagnosticExchangeTag(sessionID, epoch string) string {
	if connection == nil || connection.connection == nil || connection.peer.Validate() != nil || len(sessionID) == 0 || len(epoch) == 0 || len(sessionID) > maxIdentifierSize || len(epoch) > maxIdentifierSize {
		return ""
	}
	state := connection.connection.ConnectionState()
	if !state.HandshakeComplete {
		return ""
	}
	// Length prefixes prevent ambiguous concatenations. The original values
	// remain private; a fixed-size hash is used as the exporter context.
	hash := sha256.New()
	_, _ = hash.Write([]byte{byte(len(sessionID))})
	_, _ = hash.Write([]byte(sessionID))
	_, _ = hash.Write([]byte{byte(len(epoch))})
	_, _ = hash.Write([]byte(epoch))
	material, err := state.ExportKeyingMaterial(handoffDiagnosticLabel, hash.Sum(nil), sha256.Size)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(material)
	clear(material)
	return hex.EncodeToString(digest[:])
}
