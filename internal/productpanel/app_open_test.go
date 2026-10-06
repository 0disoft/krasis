package productpanel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

type appClient struct {
	settingsClient
	requests []localapi.AppOpenRequest
	result   localapi.AppOpenResult
	failure  bool
}

func (c *appClient) OpenApp(_ context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
	c.requests = append(c.requests, r)
	if c.failure {
		return localapi.AppOpenResult{}, errors.New("private native detail")
	}
	return c.result, nil
}

func TestAppOpenRequiresTrustAndPreservesRequestIdentity(t *testing.T) {
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	c := &appClient{result: localapi.AppOpenResult{Code: "queued", ID: id}}
	b := NewUpdateSettingsBridge(context.Background(), func() (UpdateSettingsClient, error) { return c, nil })
	if b.AppOpen(peer, "").Code != "stale_peer" || len(c.requests) != 0 {
		t.Fatal("untrusted launch")
	}
	c.peers = []localapi.TrustedPeer{{Fingerprint: peer}}
	if r := b.AppOpen(peer, ""); r.Code != "pending" || r.ID != id {
		t.Fatal(r)
	}
	c.result = localapi.AppOpenResult{Code: "launch_requested", ID: id}
	if r := b.AppOpen(peer, id); r.Code != "launch_requested" {
		t.Fatal(r)
	}
	if len(c.requests) != 2 || c.requests[0].Action != "START" || c.requests[1].Action != "STATUS" || c.requests[1].ID != id {
		t.Fatal(c.requests)
	}
	c.result.ID = strings.Repeat("c", 32)
	if b.AppOpen(peer, id).Code != "unavailable" {
		t.Fatal("accepted different receipt")
	}
	c.peers[0].Revoked = true
	if b.AppOpen(peer, "").Code != "stale_peer" || len(c.requests) != 3 {
		t.Fatal("revoked launch")
	}
	c.peers[0].Revoked = false
	c.result = localapi.AppOpenResult{ID: id, Code: "peer_unresponsive"}
	if b.AppOpen(peer, id).Code != "peer_unresponsive" {
		t.Fatal("no-response result lost at the UI boundary")
	}
}

func TestAppOpenDoesNotRetryUnknownLaunchOrExposeErrors(t *testing.T) {
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	c := &appClient{failure: true}
	c.peers = []localapi.TrustedPeer{{Fingerprint: peer}}
	b := NewUpdateSettingsBridge(context.Background(), func() (UpdateSettingsClient, error) { return c, nil })
	if r := b.AppOpen(peer, ""); r.Code != "outcome_unknown" || len(c.requests) != 1 {
		t.Fatal(r, c.requests)
	}
	if b.AppOpen(peer, id).Code != "unavailable" {
		t.Fatal("lost status reported as launch")
	}
	for _, args := range [][2]string{{"../exe", ""}, {peer, "../receipt"}} {
		if b.AppOpen(args[0], args[1]).Code != "invalid_peer" {
			t.Fatal("invalid identity accepted")
		}
	}
	if len(c.requests) != 2 {
		t.Fatal("invalid request reached API")
	}
	c.failure = false
	c.result = localapi.AppOpenResult{Code: "queued"}
	if b.AppOpen(peer, "").Code != "outcome_unknown" {
		t.Fatal("missing job accepted")
	}
}
