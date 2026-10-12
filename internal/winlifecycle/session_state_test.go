package winlifecycle

import "testing"

func TestSessionSchedulingHintTracksOnlyLockAndUnlockNotifications(t *testing.T) {
	previous := SessionLocked()
	defer currentSessionLocked.Store(previous)
	observeSessionState(wmWTSSessionChange, wtsSessionLock)
	if !SessionLocked() {
		t.Fatal("lock hint missed")
	}
	observeSessionState(wmPowerBroadcast, wtsSessionUnlock)
	if !SessionLocked() {
		t.Fatal("unrelated message cleared lock")
	}
	observeSessionState(wmWTSSessionChange, wtsSessionUnlock)
	if SessionLocked() {
		t.Fatal("unlock retained stale secure owner preference")
	}
}
