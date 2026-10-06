package productlog

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

const maxSupportRequests = 32
const maxRequestSupportBytes int64 = 128 << 20
const requestSnapshotGrace = 2 * time.Minute // Longer than the peer request lifetime.

var requestSnapshotGate sync.Mutex

// ErrRequestStorageLimit means active or recent requests fill the retention budget.
var ErrRequestStorageLimit = errors.New("diagnostic request storage limit")

type requestSnapshotGroup struct {
	id       string
	bytes    int64
	modified time.Time
}

func snapshotRequestID(name string) string {
	const prefix = "support-request-"
	if !strings.HasPrefix(name, prefix) || len(name) < len(prefix)+32 {
		return ""
	}
	id, suffix := name[len(prefix):len(prefix)+32], name[len(prefix)+32:]
	if !validRequestID(id) {
		return ""
	}
	if suffix == "-local.json" {
		return id
	}
	if strings.HasPrefix(suffix, "-peer-") && strings.HasSuffix(suffix, ".json") {
		peer := strings.TrimSuffix(strings.TrimPrefix(suffix, "-peer-"), ".json")
		if len(peer) == 64 && validRequestID(peer[:32]) && validRequestID(peer[32:]) {
			return id
		}
	}
	return ""
}

// Reserve capacity before creating a snapshot. Current and recently active
// requests are never pruned; if they fill the budget, reject the new export.
// Names come from enumeration, but inspection/removal stays handle-relative.
func reserveRequestSnapshot(dir *securefs.Directory, id, name string, size int64) error {
	if err := resumeRequestPruning(dir); err != nil {
		return err
	}
	entries, err := dir.ReadNames()
	if err != nil {
		return err
	}
	groups := map[string]*requestSnapshotGroup{}
	var total int64
	for _, entry := range entries {
		requestID := snapshotRequestID(entry)
		if requestID == "" {
			continue
		}
		f, err := dir.Open(entry)
		if err != nil {
			return err
		}
		err = peercredentials.ValidateSecurityFileHandle(f)
		info, statErr := f.Stat()
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if statErr != nil {
			return statErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !info.Mode().IsRegular() || info.Size() < 0 {
			return errors.New("invalid diagnostic snapshot metadata")
		}
		group := groups[requestID]
		if group == nil {
			group = &requestSnapshotGroup{id: requestID}
			groups[requestID] = group
		}
		group.bytes += info.Size()
		total += info.Size()
		if info.ModTime().After(group.modified) {
			group.modified = info.ModTime()
		}
		if entry == name {
			size = 0
		}
	}
	count := len(groups)
	if groups[id] == nil {
		count++
	}
	var candidates []*requestSnapshotGroup
	now := time.Now()
	for _, group := range groups {
		if group.id != id && now.Sub(group.modified) > requestSnapshotGrace {
			candidates = append(candidates, group)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modified.Equal(candidates[j].modified) {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].modified.Before(candidates[j].modified)
	})
	for _, group := range candidates {
		if count <= maxSupportRequests && total+size <= maxRequestSupportBytes {
			break
		}
		if err := markRequestPrune(dir, group.id); err != nil {
			return err
		}
		if err := finishRequestPrune(dir, group.id, dir.Remove); err != nil {
			return err
		}
		count--
		total -= group.bytes
	}
	if count > maxSupportRequests || total+size > maxRequestSupportBytes {
		return ErrRequestStorageLimit
	}
	return nil
}
