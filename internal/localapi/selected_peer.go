package localapi

import "context"

type peerFingerprintKey struct{}

// WithPeerFingerprint carries the selected identity through the control client
// and the admitted session without changing legacy alias-only CLI interfaces.
func WithPeerFingerprint(ctx context.Context, fingerprint string) context.Context {
	return context.WithValue(ctx, peerFingerprintKey{}, fingerprint)
}

func PeerFingerprint(ctx context.Context) string {
	fingerprint, _ := ctx.Value(peerFingerprintKey{}).(string)
	return fingerprint
}
