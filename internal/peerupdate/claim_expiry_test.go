package peerupdate

import "testing"

func TestClaimExpiryCannotReleaseAnotherJob(t *testing.T) {
	m := &Manager{status: Status{State: "APPLYING", ID: "first", Peer: "peer"}}
	if m.ExpireClaim("other", "peer") || m.ExpireClaim("first", "other") {
		t.Fatal("foreign timeout released the claim")
	}
	if !m.ExpireClaim("first", "peer") || m.Status().State != "APPLY_FAILED" {
		t.Fatal("abandoned claim remained applying")
	}
	m.status = Status{State: "APPLYING", ID: "second", Peer: "peer"}
	if m.ExpireClaim("first", "peer") {
		t.Fatal("stale timeout released the replacement job")
	}
}
