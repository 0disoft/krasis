package residentagent

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/truststore"
)

type connectionStore interface {
	Connection(string) (truststore.ConnectionProfile, error)
	SetConnection(string, truststore.ConnectionSettings) (truststore.ConnectionProfile, error)
}

func (operations *Operations) Connection(ctx context.Context, request localapi.ConnectionRequest) (localapi.ConnectionResult, error) {
	result := localapi.ConnectionResult{SchemaVersion: localapi.ConnectionSchemaVersion, Status: "REJECTED", Code: "invalid_request"}
	if !request.Valid() {
		return result, nil
	}
	store, ok := operations.trust.(connectionStore)
	if !ok {
		result.Code = "not_configured"
		return result, nil
	}
	// Serialize setting changes against Connect. Keep the session snapshot stable
	// even if another alias or future topology would refer to the same peer.
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if request.Action == "SET" && operations.sessionDone != nil {
		result.Code = "active_session_requires_disconnect"
		return result, nil
	}
	var profile truststore.ConnectionProfile
	var err error
	if request.Action == "SET" {
		profile, err = store.SetConnection(request.PeerAlias, truststore.ConnectionSettings{Address: request.Address, Edge: request.Edge})
	} else {
		profile, err = store.Connection(request.PeerAlias)
	}
	if err != nil {
		switch {
		case errors.Is(err, truststore.ErrStoreNotFound), errors.Is(err, truststore.ErrPeerNotFound):
			result.Code = "peer_not_found"
		case errors.Is(err, truststore.ErrAliasAmbiguous):
			result.Code = "peer_alias_ambiguous"
		case errors.Is(err, truststore.ErrConnectionNotConfigured):
			result.Code = "connection_not_configured"
		case errors.Is(err, truststore.ErrStoreBusy):
			result.Status = "FAILED"
			result.Code = "trust_store_busy"
		default:
			return result, err
		}
		return result, nil
	}
	result.Status, result.Code, result.Profile = "COMPLETED", "connection_loaded", &profile
	if request.Action == "SET" {
		result.Code = "connection_saved"
		operations.recordLocked(result.Code)
	}
	return result, nil
}
