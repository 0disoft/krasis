package productlog

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

const CollectionReceiptName = "diagnostic-collection-latest.json"

// Native-only resumable pointer. Paths are derived from validated IDs rather
// than stored or accepted from the renderer. This file is not support content.
type CollectionReceipt struct {
	Schema     string    `json:"schema"`
	ID         string    `json:"id"`
	Peer       string    `json:"peer"`
	Expires    time.Time `json:"expires"`
	State      string    `json:"state"`
	Code       string    `json:"code"`
	LocalSaved bool      `json:"localSaved"`
	PeerSaved  bool      `json:"peerSaved"`
}

func (r CollectionReceipt) valid() bool {
	if r.Schema != "krasis.diagnostic-collection/v1" {
		return false
	}
	if r.ID != "" && !validRequestID(r.ID) {
		return false
	}
	if r.Peer != "" {
		_, err := hex.DecodeString(r.Peer)
		if err != nil || len(r.Peer) != 64 || r.Peer != strings.ToLower(r.Peer) {
			return false
		}
	}
	if r.Code == "collected" && r.State != "RECEIVED" || r.Code == "pending" && r.State != "PENDING" ||
		r.Code == "local_only" && (r.State != "LOCAL_ONLY" || !r.LocalSaved) {
		return false
	}
	switch r.State {
	case "SAVING":
		return r.ID == "" && r.Peer == "" && !r.LocalSaved && !r.PeerSaved && r.Code == "saving"
	case "PENDING":
		if r.ID == "" || r.Peer == "" || r.Expires.IsZero() || !r.LocalSaved || r.PeerSaved {
			return false
		}
	case "RECEIVED":
		if r.ID == "" || r.Peer == "" || !r.LocalSaved || !r.PeerSaved || r.Code != "collected" {
			return false
		}
	case "FAILED", "UNKNOWN", "EXPIRED", "LOCAL_ONLY":
		if r.PeerSaved {
			return false
		}
	default:
		return false
	}
	switch r.Code {
	case "pending", "collected", "partial", "local_only", "save_failed", "cancelled", "unsupported", "stale_peer", "outcome_unknown", "rejected", "busy", "storage_limit", "state_save_failed":
		return true
	}
	return false
}

func SaveCollectionReceipt(base string, r CollectionReceipt) error {
	if !r.valid() {
		return errors.New("invalid diagnostic collection receipt")
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	f, temp, err := dir.CreateTemporary(".collection-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(temp)
	defer f.Close()
	if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
		return err
	}
	if err = json.NewEncoder(f).Encode(r); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return dir.Replace(temp, CollectionReceiptName)
}

func LoadCollectionReceipt(base string) (*CollectionReceipt, error) {
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return nil, err
	}
	dir, err := securefs.OpenReadDirectory(paths.Diagnostics)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	if err = peercredentials.ValidateSecurityDirectoryHandle(dir.Handle()); err != nil {
		return nil, err
	}
	f, err := dir.Open(CollectionReceiptName)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = peercredentials.ValidateSecurityFileHandle(f); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(data) > 4096 {
		return nil, errors.New("invalid diagnostic collection receipt")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var r CollectionReceipt
	if d.Decode(&r) != nil || !r.valid() || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid diagnostic collection receipt")
	}
	// Recheck the actual immutable files before restoring any saved indicator.
	if r.LocalSaved {
		name := ExportName
		if r.ID != "" {
			name = "support-request-" + r.ID + "-local.json"
		}
		if _, err = readSupportSnapshot(dir, name); err != nil {
			return nil, err
		}
	}
	if r.PeerSaved {
		if _, err = readSupportSnapshot(dir, "support-request-"+r.ID+"-peer-"+r.Peer+".json"); err != nil {
			return nil, err
		}
	}
	return &r, nil
}

// CollectionLocalPath returns only a validated native-derived file for Explorer.
func CollectionLocalPath(base string) (string, error) {
	r, err := LoadCollectionReceipt(base)
	if err != nil || r == nil || !r.LocalSaved {
		return "", err
	}
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return "", err
	}
	name := ExportName
	if r.ID != "" {
		name = "support-request-" + r.ID + "-local.json"
	}
	return filepath.Join(paths.Diagnostics, name), nil
}
