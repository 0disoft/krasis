package productpanel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/pindiscovery"
)

type pairingStub struct {
	state      string
	generation int
	pair       localapi.OperationResult
	req        localapi.OperationRequest
	calls      int
	err        error
}

func (s *pairingStub) Status(context.Context) (localapi.Status, error) {
	return localapi.Status{State: s.state}, nil
}
func (s *pairingStub) Trust(context.Context) (localapi.Trust, error) {
	return localapi.Trust{Peers: []localapi.TrustedPeer{}}, nil
}
func (s *pairingStub) Runtime(context.Context) (localapi.RuntimeInfo, error) {
	return localapi.RuntimeInfo{Generation: s.generation}, nil
}
func (s *pairingStub) Pair(_ context.Context, req localapi.OperationRequest) (localapi.OperationResult, error) {
	s.calls++
	s.req = req
	return s.pair, s.err
}
func (s *pairingStub) Connect(context.Context, string) (localapi.OperationResult, error) {
	return localapi.OperationResult{Status: "COMPLETED"}, nil
}

func TestPairingKeepsAddressesAndInvalidPinsOutOfSnapshots(t *testing.T) {
	id := strings.Repeat("a", 32)
	stub := &pairingStub{state: "LOCAL", generation: 3, pair: localapi.OperationResult{Status: "COMPLETED", Code: "pin_waiting", PairingID: id, Confirmation: "123456"}}
	bridge := NewPairingBridge(context.Background(), func() (PairingClient, error) { return stub, nil }, func(context.Context) ([]pindiscovery.Candidate, error) {
		return []pindiscovery.Candidate{{Address: "192.168.1.9:45871", ID: id}}, nil
	})
	invited := bridge.Invite()
	if invited.PIN != "123456" || invited.Job != id || stub.req.Address != "" || stub.req.PairingAction != "PIN_LISTEN" {
		t.Fatalf("invite = %+v req=%+v", invited, stub.req)
	}
	stub.pair = localapi.OperationResult{Status: "COMPLETED", Code: "pairing_cancelled", PairingID: id}
	if cancelled := bridge.Cancel(); cancelled.Job != "" || cancelled.PIN != "" {
		t.Fatal("cancel retained invitation")
	}
	searched := bridge.Search()
	raw, _ := json.Marshal(searched)
	if strings.Contains(string(raw), "192.168") || strings.Contains(string(raw), "45871") || searched.Candidates[0].ID != id {
		t.Fatal(string(raw))
	}
	if bridge.Join("missing", "123456").Code != "stale_candidate" || stub.calls != 2 {
		t.Fatal("unknown candidate reached the agent")
	}
	if bridge.Join(id, "12ab").Code != "invalid_pin" {
		t.Fatal("invalid pin accepted")
	}
	stub.pair = localapi.OperationResult{Status: "COMPLETED", Code: "pairing_completed", PairingID: id, Confirmation: "999999"}
	joined := bridge.Join(id, "654321")
	if stub.req.Address != "192.168.1.9:45871" || stub.req.ConfirmationCode != "654321" || joined.PIN != "" || joined.Alias != "PC-"+id[:8] {
		t.Fatalf("join = %+v req=%+v", joined, stub.req)
	}
}

func TestPairingRetainsJobAfterUncertainPollAndPreventsReplacement(t *testing.T) {
	id := strings.Repeat("b", 32)
	stub := &pairingStub{state: "LOCAL", generation: 32, pair: localapi.OperationResult{Status: "COMPLETED", Code: "pin_waiting", PairingID: id, Confirmation: "123456"}}
	bridge := NewPairingBridge(context.Background(), func() (PairingClient, error) { return stub, nil }, nil)
	bridge.Invite()
	for _, result := range []PairingSnapshot{bridge.Invite(), bridge.Search(), bridge.Join(id, "123456")} {
		if result.Code != "pairing_already_active" || result.Job != id || result.PIN != "" {
			t.Fatalf("active result = %+v", result)
		}
	}
	if stub.calls != 1 {
		t.Fatal("new operation replaced pending job")
	}
	stub.err = errors.New("private endpoint details")
	if got := bridge.Status(); got.Job != id || got.Code != "unavailable" || got.PIN != "" {
		t.Fatalf("uncertain result = %+v", got)
	}
	stub.err = nil
	stub.pair = localapi.OperationResult{Status: "COMPLETED", Code: "pairing_completed", PairingID: id}
	if got := bridge.Status(); got.Job != "" || got.Alias != "PC-"+id[:8] || stub.req.PairingID != id {
		t.Fatalf("completed result = %+v", got)
	}
	if got := bridge.Status(); got.Code != "ok" || got.Alias != "" {
		t.Fatal("completion replayed")
	}
}

func TestPairingConcurrentStatusAndInvite(t *testing.T) {
	stub := &pairingStub{state: "LOCAL", generation: 32, pair: localapi.OperationResult{Status: "COMPLETED", Code: "pin_waiting", PairingID: strings.Repeat("c", 32), Confirmation: "123456"}}
	bridge := NewPairingBridge(context.Background(), func() (PairingClient, error) { return stub, nil }, nil)
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() { defer group.Done(); bridge.Invite(); bridge.Status() }()
	}
	group.Wait()
	if got := bridge.Status(); got.Code != "pin_waiting" {
		t.Fatalf("status = %+v", got)
	}
}

func TestPairingRejectsNonLocalAndOldAgents(t *testing.T) {
	stub := &pairingStub{state: "CONNECTED", generation: 32}
	bridge := NewPairingBridge(context.Background(), func() (PairingClient, error) { return stub, nil }, nil)
	if bridge.Invite().Code != "not_local" || stub.calls != 0 {
		t.Fatal("connected session started pairing")
	}
	stub.state, stub.generation = "LOCAL", 2
	if bridge.Invite().Code != "agent_incompatible" || stub.calls != 0 {
		t.Fatal("old agent started pairing")
	}
}
