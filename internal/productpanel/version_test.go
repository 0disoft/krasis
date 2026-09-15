package productpanel

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
	"strings"
	"testing"
	"testing/synctest"
)

type versionClient struct {
	RuntimeClient
	info localapi.RuntimeInfo
	err  error
	wait bool
}

func (c versionClient) Runtime(ctx context.Context) (localapi.RuntimeInfo, error) {
	if c.wait {
		<-ctx.Done()
		return localapi.RuntimeInfo{}, ctx.Err()
	}
	return c.info, c.err
}

func TestBuildLabelAcceptsOnlyBoundedBuildIdentifiers(t *testing.T) {
	for _, test := range []struct{ raw, want string }{
		{strings.Repeat("a", 40), "aaaaaaaa"}, {strings.Repeat("b", 64), "bbbbbbbb"},
		{"development", "개발 빌드"}, {"", "확인 불가"}, {"private-path\nmarker", "확인 불가"}, {strings.Repeat("z", 40), "확인 불가"},
	} {
		if got := BuildLabel(test.raw); got != test.want {
			t.Fatalf("got=%q want=%q", got, test.want)
		}
	}
}

func TestAgentVersionIsFreshBoundedAndFailureIsNonfatal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, test := range []struct {
			client versionClient
			want   string
		}{
			{versionClient{info: localapi.RuntimeInfo{Generation: 11, Build: strings.Repeat("a", 40)}}, "g11 · aaaaaaaa"},
			{versionClient{info: localapi.RuntimeInfo{Generation: 12, Build: "development"}}, "g12 · 개발 빌드"},
			{versionClient{err: errors.New("private-error")}, "확인 불가"},
			{versionClient{wait: true}, "확인 불가"},
		} {
			got := AgentVersionLabel(context.Background(), func() (RuntimeClient, error) { return test.client, nil })
			if got != test.want {
				t.Fatalf("got=%q want=%q", got, test.want)
			}
		}
	})
}
