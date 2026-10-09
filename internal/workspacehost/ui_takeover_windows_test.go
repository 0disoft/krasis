//go:build windows && desktop

package workspacehost

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productpanel"
)

type takeoverPlan struct {
	stopped, closed bool
	stopErr         error
}

func (p *takeoverPlan) HasProcesses() bool         { return true }
func (p *takeoverPlan) Close() error               { p.closed = true; return nil }
func (p *takeoverPlan) Stop(context.Context) error { p.stopped = true; return p.stopErr }

func TestIncomingUITakeoverVerifiesBundleAndStableAgentBeforeClosingOldWindow(t *testing.T) {
	for _, scenario := range []string{"ready", "wrong_agent", "wrong_build", "foreign_ui", "bad_bundle", "agent_changed", "cancelled", "exit_failed"} {
		t.Run(scenario, func(t *testing.T) {
			revision := strings.Repeat("a", 40)
			info := localapi.RuntimeInfo{Protocol: localapi.AgentProtocol, Generation: 256, Build: revision, Instance: strings.Repeat("b", 32)}
			plan := &takeoverPlan{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls, prepared, verified := 0, false, false
			if scenario == "wrong_agent" {
				info.Generation--
			}
			if scenario == "wrong_build" {
				info.Build = strings.Repeat("c", 40)
			}
			if scenario == "exit_failed" {
				plan.stopErr = context.DeadlineExceeded
			}
			replaced, err := takeOverMatchedUI(ctx, 256, revision, uiTakeoverOps{
				runtime: func(context.Context) (localapi.RuntimeInfo, error) {
					calls++
					value := info
					if scenario == "agent_changed" && calls > 1 {
						value.Instance = strings.Repeat("d", 32)
					}
					return value, nil
				},
				prepare: func() (olderUIPlan, error) {
					prepared = true
					if scenario == "foreign_ui" {
						return nil, productpanel.ErrAgentIncompatible
					}
					return plan, nil
				},
				verify: func() error {
					verified = true
					if scenario == "bad_bundle" {
						return productpanel.ErrAgentIncompatible
					}
					if scenario == "cancelled" {
						cancel()
					}
					return nil
				},
			})
			if scenario == "ready" {
				if !replaced || err != nil || !plan.stopped || !plan.closed || !verified {
					t.Fatal(replaced, err, plan)
				}
			} else if scenario == "wrong_agent" || scenario == "wrong_build" {
				if replaced || err != nil || prepared || plan.stopped {
					t.Fatal("unmatched agent triggered takeover", err)
				}
			} else {
				if replaced || err == nil || scenario != "exit_failed" && plan.stopped {
					t.Fatal("unverified or cancelled takeover closed old window", err, plan)
				}
				if scenario != "foreign_ui" && !plan.closed {
					t.Fatal("retained process handle leaked")
				}
				if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}
		})
	}
}
