package inputworker

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestPresentationConsumesOnlyRecentCleanReturnOfSameProcess(t *testing.T) {
	now := time.Now()
	lease := Lease{Identity: application.LeaseIdentity{SessionID: "session", LeaseID: "lease", Epoch: "epoch"}, Binding: Binding{1}, ExpiresAt: now.Add(time.Minute)}
	presentation := lease
	presentation.Presentation = true
	presentation.ExpiresAt = now.Add(30 * time.Second)
	for _, kind := range []string{"ordinary", "other pid", "other lifetime", "other lease", "expired receipt", "renewed expiry", "missing receipt"} {
		r := presentationReturn{lease: lease, pid: 7, started: 11, at: now}
		request, pid := presentation, uint32(7)
		switch kind {
		case "ordinary":
			request.Presentation = false
		case "other pid":
			pid = 8
		case "other lifetime":
			r.started = 12
		case "other lease":
			request.Identity.LeaseID = "other"
		case "expired receipt":
			r.at = now.Add(-2 * time.Second)
		case "renewed expiry":
			request.ExpiresAt = lease.ExpiresAt.Add(time.Second)
		case "missing receipt":
			r = presentationReturn{}
		}
		if r.take(request, pid, 11, now) {
			t.Fatal("unauthorized presentation", kind)
		}
	}
	r := presentationReturn{lease: lease, pid: 7, started: 11, at: now}
	if !r.take(presentation, 7, 11, now) || r.take(presentation, 7, 11, now) {
		t.Fatal("presentation receipt replayed")
	}
}

func TestPresentationEnvelopeCannotBecomeInputAuthority(t *testing.T) {
	now := time.Now()
	lease := Lease{Identity: application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, Binding: Binding{1}, ExpiresAt: now.Add(time.Second), Presentation: true, CursorX: -20, CursorY: 300}
	packet, err := encodeLease(lease, now)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeLease(packet, now)
	if err != nil || !decoded.Presentation || decoded.CursorX != -20 || decoded.CursorY != 300 {
		t.Fatal("presentation metadata lost", err)
	}
	if _, err := NewClient(context.Background(), nil, decoded); err == nil {
		t.Fatal("presentation constructed input client")
	}
	copy(packet[:4], "KRL1")
	if _, err := decodeLease(packet, now); err == nil {
		t.Fatal("presentation accepted as ordinary lease")
	}
	lease.ExpiresAt = now.Add(time.Minute + time.Second)
	if _, err := encodeLease(lease, now); err == nil {
		t.Fatal("presentation budget renewed")
	}
}
