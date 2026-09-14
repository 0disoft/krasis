package productpanel

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
)

// SetLayout changes only this PC's exit edge, without re-pairing or replacing an address.
func SetLayout(ctx context.Context, open func() (Client, error), fingerprint, edge string) View {
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
	if view.State != "LOCAL" || alias == "" || (edge != "LEFT" && edge != "RIGHT") {
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
	result, err := editor.Connection(ctx, localapi.ConnectionRequest{Action: "SET_EDGE", PeerAlias: alias, Fingerprint: fingerprint, Edge: edge})
	if err != nil || result.Status != "COMPLETED" || result.Code != "connection_saved" {
		view.Notice = "배치 저장을 확인하지 못했습니다. 새로고침하거나 기존 연결 설정을 확인하세요."
		return view
	}
	view.Notice = "이 PC의 배치를 저장했습니다. 상대 PC에서도 반대 방향을 선택한 뒤 연결하세요."
	return view
}
