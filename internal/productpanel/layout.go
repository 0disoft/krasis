package productpanel

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
)

// SetLayout changes only this PC's exit edge, without re-pairing or replacing an address.
func SetLayout(ctx context.Context, open func() (Client, error), fingerprint, edge string) View {
	return setBoundary(ctx, open, fingerprint, edge, 0)
}

func SetRearm(ctx context.Context, open func() (Client, error), fingerprint string, distance int32) View {
	return setBoundary(ctx, open, fingerprint, "", distance)
}

func setBoundary(ctx context.Context, open func() (Client, error), fingerprint, edge string, distance int32) View {
	c, err := open()
	if err != nil {
		return View{Notice: "에이전트에 접근할 수 없습니다."}
	}
	current := func() (Client, error) { return c, nil }
	view := Execute(ctx, current, "refresh", "")
	alias := ""
	for name, id := range view.Fingerprints {
		if id == fingerprint && len(id) == 64 {
			alias = name
		}
	}
	valid := (distance == 0 && (edge == "LEFT" || edge == "RIGHT")) || (edge == "" && (distance == 32 || distance == 64 || distance == 128))
	if view.State != "LOCAL" || alias == "" || !valid {
		view.Notice = "연결 해제 후 기기를 다시 선택하세요."
		return view
	}
	editor, ok := c.(interface {
		Connection(context.Context, localapi.ConnectionRequest) (localapi.ConnectionResult, error)
	})
	if !ok {
		view.Notice = "새로고침으로 에이전트를 업데이트하세요."
		return view
	}
	request := localapi.ConnectionRequest{Action: "SET_EDGE", PeerAlias: alias, Fingerprint: fingerprint, Edge: edge}
	if distance != 0 {
		request.Action = "SET_REARM"
		request.RearmDistance = distance
	}
	result, err := editor.Connection(ctx, request)
	if err != nil || result.Status != "COMPLETED" || result.Code != "connection_saved" {
		view.Notice = "배치 저장을 확인하지 못했습니다. 새로고침하거나 기존 연결 설정을 확인하세요."
		return view
	}
	view.Notice = "이 PC의 배치를 저장했습니다. 상대 PC에서도 반대 방향을 선택한 뒤 연결하세요."
	if distance != 0 {
		view.Notice = "이 PC의 경계 재진입 여유를 저장했습니다. 다음 연결부터 적용됩니다."
	}
	return view
}

type LayoutSnapshot struct {
	Edge  string `json:"edge"`
	Rearm int32  `json:"rearm"`
	Code  string `json:"code"`
}

func layoutFailure(code string) LayoutSnapshot {
	return LayoutSnapshot{Edge: "RIGHT", Rearm: 32, Code: code}
}

func LayoutStatus(ctx context.Context, open func() (Client, error), fingerprint string) LayoutSnapshot {
	if len(fingerprint) != 64 {
		return layoutFailure("invalid_peer")
	}
	c, err := open()
	if err != nil {
		return layoutFailure("unavailable")
	}
	view := Execute(ctx, func() (Client, error) { return c, nil }, "refresh", "")
	alias := ""
	for name, id := range view.Fingerprints {
		if id == fingerprint {
			alias = name
		}
	}
	if alias == "" {
		return layoutFailure("invalid_peer")
	}
	editor, ok := c.(interface {
		Connection(context.Context, localapi.ConnectionRequest) (localapi.ConnectionResult, error)
	})
	if !ok {
		return layoutFailure("unavailable")
	}
	got, err := editor.Connection(ctx, localapi.ConnectionRequest{Action: "SHOW", PeerAlias: alias})
	if err != nil {
		return layoutFailure("unavailable")
	}
	if got.Code == "connection_not_configured" {
		return layoutFailure("unconfigured")
	}
	if got.Status != "COMPLETED" || got.Profile == nil || (got.Profile.Edge != "LEFT" && got.Profile.Edge != "RIGHT") {
		return layoutFailure("rejected")
	}
	return LayoutSnapshot{Edge: got.Profile.Edge, Rearm: got.Profile.Rearm(), Code: "ok"}
}

func LayoutSet(ctx context.Context, open func() (Client, error), fingerprint, edge string) LayoutSnapshot {
	return writeLayout(ctx, open, fingerprint, edge, 0)
}

func LayoutRearm(ctx context.Context, open func() (Client, error), fingerprint string, distance int32) LayoutSnapshot {
	return writeLayout(ctx, open, fingerprint, "", distance)
}

func writeLayout(ctx context.Context, open func() (Client, error), fingerprint, edge string, distance int32) LayoutSnapshot {
	c, err := open()
	if err != nil {
		return layoutFailure("unavailable")
	}
	view := Execute(ctx, func() (Client, error) { return c, nil }, "refresh", "")
	alias := ""
	for name, id := range view.Fingerprints {
		if id == fingerprint && len(id) == 64 {
			alias = name
		}
	}
	valid := (distance == 0 && (edge == "LEFT" || edge == "RIGHT")) || (edge == "" && (distance == 32 || distance == 64 || distance == 128))
	if alias == "" || !valid {
		return layoutFailure("invalid_peer")
	}
	if view.State != "LOCAL" {
		status := LayoutStatus(ctx, func() (Client, error) { return c, nil }, fingerprint)
		status.Code = "not_local"
		return status
	}
	editor, ok := c.(interface {
		Connection(context.Context, localapi.ConnectionRequest) (localapi.ConnectionResult, error)
	})
	if !ok {
		return layoutFailure("unavailable")
	}
	request := localapi.ConnectionRequest{Action: "SET_EDGE", PeerAlias: alias, Fingerprint: fingerprint, Edge: edge}
	if distance != 0 {
		request.Action = "SET_REARM"
		request.RearmDistance = distance
	}
	got, err := editor.Connection(ctx, request)
	if err != nil {
		return layoutFailure("unavailable")
	}
	if got.Status != "COMPLETED" || got.Code != "connection_saved" || got.Profile == nil {
		if got.Code == "connection_not_configured" {
			return layoutFailure("unconfigured")
		}
		return layoutFailure("rejected")
	}
	return LayoutSnapshot{Edge: got.Profile.Edge, Rearm: got.Profile.Rearm(), Code: "ok"}
}
