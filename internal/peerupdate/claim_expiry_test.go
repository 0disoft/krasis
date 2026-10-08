package peerupdate

import "testing"

func TestRejectReadyCannotFailAnActiveOrForeignJob(t *testing.T) {
	for _, state := range []string{"READY", "APPLYING", "STAGED", "UPDATED"} {
		m := &Manager{status: Status{State: state, ID: "first", Peer: "peer"}}
		if m.RejectReady("other", "peer") || m.RejectReady("first", "other") {
			t.Fatal("foreign ready job rejected")
		}
		if got := m.RejectReady("first", "peer"); got != (state == "READY") {
			t.Fatal("wrong claim state rejected", state)
		}
		if state == "READY" && m.Status().State != "APPLY_FAILED" {
			t.Fatal("rejection not visible")
		}
	}
}

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
