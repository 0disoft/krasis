package residentagent

import (
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/application"
)

type testCursorClient struct {
	testPrivilegedClient
	sets, reads, moves int
}

func (p *testCursorClient) CursorPosition(context.Context) (int32, int32, error) {
	p.reads++
	return 10, 20, p.err
}
func (p *testCursorClient) SetCursorPosition(context.Context, int32, int32) error {
	p.sets++
	return p.err
}
func (p *testCursorClient) MoveWithPosition(context.Context, int32, int32) (int32, int32, error) {
	p.moves++
	return 11, 22, p.err
}

func TestPrivilegedCursorUsesSelectedClientAndRefusesClosedOrMissingCapability(t *testing.T) {
	p := &testCursorClient{}
	input := &privilegedLeaseInput{client: p}
	ctx := context.Background()
	x, y, err := input.CursorPosition(ctx)
	if err != nil || x != 10 || y != 20 {
		t.Fatal("protected cursor read lost")
	}
	if err := input.SetCursorPosition(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	x, y, err = input.MoveWithPosition(ctx, 1, 2)
	if err != nil || x != 11 || y != 22 || p.reads != 1 || p.sets != 1 || p.moves != 1 {
		t.Fatal("protected motion snapshot not delegated")
	}
	for _, client := range []privilegedClient{nil, &testPrivilegedClient{}} {
		input.client = client
		_, _, err = input.CursorPosition(ctx)
		if certainty, known := application.InjectionCertaintyOf(err); !known || certainty != application.InjectionNotApplied {
			t.Fatal("missing cursor capability invented success")
		}
		if err := input.SetCursorPosition(ctx, 1, 2); err == nil {
			t.Fatal("cursor set without selected client succeeded")
		}
	}
}
