package productpanel

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
)

// Revoke binds the user's confirmation to a fingerprint, not a mutable alias.
func Revoke(ctx context.Context, open func() (Client, error), fingerprint string) View {
	client, err := open()
	if err != nil {
		return View{Notice: "에이전트에 접근할 수 없습니다."}
	}
	current := func() (Client, error) { return client, nil }
	view := Execute(ctx, current, "refresh", "")
	known := false
	for _, value := range view.Fingerprints {
		if value == fingerprint && len(value) == 64 {
			known = true
		}
	}
	if view.State != "LOCAL" || !known {
		view.Notice = "기기 목록이 바뀌었거나 연결 중입니다. 연결 해제 후 다시 선택하세요."
		return view
	}
	revoker, ok := client.(interface {
		RevokeTrust(context.Context, string) (localapi.OperationResult, error)
	})
	if !ok {
		view.Notice = "이 에이전트는 등록 해제를 지원하지 않습니다."
		return view
	}
	result, err := revoker.RevokeTrust(ctx, fingerprint)
	if err != nil || result.Status != "COMPLETED" || result.Code != "trust_revoked" {
		view.Notice = "등록 해제를 확인하지 못했습니다. 새로고침으로 확인하세요."
		return view
	}
	view = Execute(ctx, current, "refresh", "")
	view.Notice = "이 PC에서 상대 기기의 등록을 해제했습니다. 다시 사용하려면 재등록하세요."
	return view
}
