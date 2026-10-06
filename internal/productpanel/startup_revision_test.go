package productpanel

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

type revisionRuntime struct {
	runtimeStub
	build string
}

func (r *revisionRuntime) Runtime(context.Context) (localapi.RuntimeInfo, error) {
	return localapi.RuntimeInfo{Protocol: 1, Generation: r.generation, Build: r.build, Instance: "instance"}, nil
}

func TestStartupNeverSilentlyAcceptsAnotherProductionBuild(t *testing.T) {
	for _, tc := range []struct {
		generation int
		build      string
		compatible bool
	}{{2, "expected", true}, {2, "other", false}, {3, "expected", false}} {
		r := &revisionRuntime{runtimeStub: runtimeStub{generation: tc.generation, state: "LOCAL"}, build: tc.build}
		s := Startup{Generation: 2, Revision: "expected", Lock: func() (io.Closer, error) { return closerStub{}, nil }, Open: func() (RuntimeClient, error) { return r, nil }}
		err := s.Ensure(context.Background())
		if tc.compatible && err != nil || !tc.compatible && !errors.Is(err, ErrAgentIncompatible) || r.stops != 0 {
			t.Fatal("unexpected build accepted or stopped", err, r.stops)
		}
	}
}
