//go:build windows

package inputworker

import "context"

// Files may be replaced after a clean stop and before the candidate ever runs.
// Only a clean receipt bound to a verified transaction image can select a
// retired/rollback image. The journal itself is never execution authority.
func stoppedInstallDigest(ctx context.Context, journal *installJournal, record serviceRecord, images func(installDirectory) (installImages, bool, error)) ([32]byte, error) {
	if ctx.Err() != nil || record.Phase != serviceClean || journal != nil && !journal.valid() {
		return [32]byte{}, ErrRecovery
	}
	if _, err := encodeServiceRecord(record); err != nil {
		return [32]byte{}, ErrRecovery
	}
	for _, kind := range []installDirectory{installActive, installRetired, installBackup} {
		if journal == nil && kind != installActive {
			continue
		}
		actual, present, err := images(kind)
		if err != nil {
			return [32]byte{}, err
		}
		if !present {
			continue
		}
		if !actual.valid() || journal != nil && actual != journal.Target && (!journal.Previous.Present || actual != journal.Previous.Images) {
			return [32]byte{}, ErrRecovery
		}
		if actual.Service == record.Digest {
			return actual.Service, nil
		}
	}
	return [32]byte{}, ErrRecovery
}
