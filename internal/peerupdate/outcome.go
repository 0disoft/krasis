package peerupdate

import (
	"errors"
	"os"
	"runtime/debug"
)

var (
	ErrSameVersion        = errors.New("same update build")
	ErrNewerVersion       = errors.New("receiver is newer")
	ErrGenerationConflict = errors.New("same generation with different build")
	ErrHashMismatch       = errors.New("archive hash mismatch")
	ErrStorage            = errors.New("update staging failed")
	ErrTransfer           = errors.New("update transfer interrupted")
)

func FailureCode(err error) string {
	switch {
	case errors.Is(err, ErrSameVersion):
		return "SAME_VERSION"
	case errors.Is(err, ErrNewerVersion):
		return "PEER_NEWER"
	case errors.Is(err, ErrGenerationConflict):
		return "GENERATION_CONFLICT"
	case errors.Is(err, ErrDenied):
		return "PEER_NOT_ALLOWED"
	case errors.Is(err, ErrBusy):
		return "PEER_BUSY"
	case errors.Is(err, ErrHashMismatch):
		return "HASH_MISMATCH"
	case errors.Is(err, ErrStorage):
		return "STORAGE_FAILED"
	case errors.Is(err, ErrTransfer):
		return "TRANSFER_INTERRUPTED"
	case errors.Is(err, os.ErrNotExist):
		return "BUNDLE_MISSING"
	default:
		return "INVALID_BUNDLE"
	}
}

func wireResult(code string) string {
	switch code {
	case "DELIVERED", "PEER_REJECTED", "SAME_VERSION", "PEER_NEWER", "GENERATION_CONFLICT", "PEER_NOT_ALLOWED", "PEER_BUSY", "HASH_MISMATCH", "STORAGE_FAILED", "TRANSFER_INTERRUPTED", "INVALID_BUNDLE":
		return code
	}
	return "PEER_REJECTED"
}

func buildRevision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && validID(setting.Value, 40) {
				return setting.Value
			}
		}
	}
	return ""
}

func compareVersion(generation int, revision string, targetGeneration int, targetRevision string) error {
	if generation > targetGeneration {
		return ErrNewerVersion
	}
	if generation == targetGeneration {
		if revision != "" && revision == targetRevision {
			return ErrSameVersion
		}
		return ErrGenerationConflict
	}
	return nil
}
