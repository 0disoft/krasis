//go:build linux

package agentinstance

const watchName = `Local\Krasis-Session-Watch-v1`

// AcquireWatch owns the opt-in Linux session supervisor independently from the
// resident agent lease. It permits exactly one watcher for the current user.
func AcquireWatch() (*Lease, error) { return acquire(watchName) }
