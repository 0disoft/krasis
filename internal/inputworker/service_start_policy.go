//go:build windows

package inputworker

const installServiceStartArgument = "--install-transaction"

// Boot may run only a committed image or an already restored previous image.
// An uncommitted candidate requires an installer-only SCM start argument;
// ordinary users retain query-only service access and cannot submit it.
func serviceStartAllowed(args []string, images installImages, journal *installJournal) bool {
	if !images.valid() || len(args) < 1 || args[0] != ServiceName {
		return false
	}
	transaction := len(args) == 2 && args[1] == installServiceStartArgument
	if len(args) != 1 && !transaction {
		return false
	}
	if journal == nil {
		return !transaction
	}
	if !journal.valid() {
		return false
	}
	switch journal.Phase {
	case installStart:
		return transaction && images == journal.Target
	case installCommit:
		return !transaction && images == journal.Target
	case installRollbackStart, installRollbackCommit:
		return !transaction && journal.Previous.Present && images == journal.Previous.Images
	default:
		return false
	}
}
