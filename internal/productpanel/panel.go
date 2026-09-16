// Package productpanel owns the desktop control projection, never input processing.
package productpanel

import (
	"context"
	"sort"
	"strings"

	"github.com/0disoft/krasis/internal/localapi"
)

type Client interface {
	Status(context.Context) (localapi.Status, error)
	Trust(context.Context) (localapi.Trust, error)
	Connect(context.Context, string) (localapi.OperationResult, error)
	Disconnect(context.Context) (localapi.OperationResult, error)
}

type View struct {
	Update        localapi.UpdateResult
	ExitForUpdate bool
	UpdateResume  string
	AgentVersion  string
	Fingerprints  map[string]string
	State         string
	Peers         []string
	Notice        string
}

// Execute opens fresh endpoint credentials on each action, including after an agent restart.
// Failure messages deliberately exclude endpoint paths, tokens and transport errors.
func Execute(ctx context.Context, open func() (Client, error), action, alias string) View {
	client, err := open()
	if err != nil {
		return View{Notice: "에이전트에 접근할 수 없습니다. 새로고침을 눌러 에이전트를 준비하세요."}
	}
	status, err := client.Status(ctx)
	if err != nil {
		return View{Notice: "에이전트 상태를 확인하지 못했습니다. 새로고침으로 다시 확인하세요."}
	}
	// Stopping a session must not depend on reading a possibly damaged trust store.
	faultNotice := "입력 제어 오류로 연결을 차단했습니다. ‘에이전트 재시작’으로 복구하거나 허용한 기기에서 업데이트를 보내세요."
	if status.State == "FAULTED" && action != "refresh" {
		return View{State: status.State, Notice: faultNotice}
	}
	if action == "disconnect" {
		result, err := client.Disconnect(ctx)
		if err != nil || result.Status != "COMPLETED" {
			return View{State: status.State, Notice: "연결 해제 결과를 확인하지 못했습니다. 상태를 다시 확인하세요."}
		}
		current, err := client.Status(ctx)
		if err != nil {
			return View{Notice: "연결 해제 후 상태를 확인하지 못했습니다. 새로고침하세요."}
		}
		return View{State: current.State, Notice: "연결 해제 요청을 처리했습니다."}
	}
	trust, err := client.Trust(ctx)
	if err != nil {
		if status.State == "FAULTED" {
			return View{State: status.State, Notice: faultNotice}
		}
		return View{State: status.State, Notice: "기기 목록을 읽지 못했습니다. 연결을 시작하지 않습니다."}
	}
	view := View{State: status.State, Fingerprints: map[string]string{}}
	if status.State == "FAULTED" {
		view.Notice = faultNotice
	}
	counts := map[string]int{}
	for _, peer := range trust.Peers {
		if !peer.Revoked && peer.Alias != "" && !strings.ContainsRune(peer.Alias, 0) {
			counts[peer.Alias]++
		}
	}
	for name, count := range counts {
		if count == 1 {
			view.Peers = append(view.Peers, name)
		}
	}
	sort.Strings(view.Peers)
	for _, peer := range trust.Peers {
		if !peer.Revoked && counts[peer.Alias] == 1 {
			view.Fingerprints[peer.Alias] = peer.Fingerprint
		}
	}
	var result localapi.OperationResult
	switch action {
	case "refresh":
		return view
	case "connect":
		if status.State != "LOCAL" || counts[alias] != 1 {
			view.Notice = "연결 가능한 기기를 선택하고 현재 연결을 먼저 해제하세요."
			return view
		}
		result, err = client.Connect(ctx, alias)
	default:
		view.Notice = "지원하지 않는 동작입니다."
		return view
	}
	if err != nil {
		view.Notice = "요청 결과를 확인하지 못했습니다. 다시 실행하기 전에 새로고침하세요."
		return view
	}
	if result.Status != "COMPLETED" {
		view.Notice = "요청이 완료되지 않았습니다. 기존 연결 설정과 에이전트 상태를 확인하세요."
	} else {
		view.Notice = "요청이 처리됐습니다. 연결 상태를 확인하세요."
	}
	current, err := client.Status(ctx)
	if err != nil {
		view.State = ""
		view.Notice = "요청 후 상태를 확인하지 못했습니다. 새로고침하세요."
	} else {
		view.State = current.State
	}
	return view
}

// The explicit disconnect action becomes process recovery only for a faulted
// view. Periodic refresh must never trigger a restart loop.
func RecoveryAction(state, action string) string {
	if state == "FAULTED" && action == "disconnect" {
		return "prepare"
	}
	return action
}

func DisconnectLabel(state string) string {
	if state == "FAULTED" {
		return "에이전트 재시작"
	}
	return "연결 해제"
}
