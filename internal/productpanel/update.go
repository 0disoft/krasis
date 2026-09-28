package productpanel

import (
	"fmt"
	"github.com/0disoft/krasis/internal/peerupdate"
)

// Delivery acknowledges stored bytes, not a running replacement agent.
func CanResumeAfterUpdate(localState string, status peerupdate.Status) bool {
	return localState == "LOCAL" && status.State == "UPDATED"
}

func UpdateStatusNotice(status peerupdate.Status) string {
	text := UpdateNotice(status.State)
	if text == "" {
		return ""
	}
	if status.State == "SENDING" && status.Total > 0 && status.Bytes >= 0 && status.Bytes <= status.Total {
		text = fmt.Sprintf("업데이트 전송 중 · %d%% (상대 수신 확인 전)", status.Bytes*100/status.Total)
	}
	if status.Generation > 0 && status.PeerGeneration > 0 {
		text += fmt.Sprintf(" · 업데이트 묶음 g%d / 연결 상대 g%d", status.Generation, status.PeerGeneration)
	}
	return text
}

func UpdateNotice(state string) string {
	switch state {
	case "PEER_UNREACHABLE":
		return "상대 관리 서비스가 응답하지 않습니다. 전원·네트워크·에이전트·방화벽 또는 구버전 여부를 확인하세요."
	case "PEER_UNSUPPORTED":
		return "상대 버전이 독립 업데이트 확인을 지원하지 않습니다. 처음 한 번 수동 교체가 필요합니다."
	case "SENDING":
		return "업데이트 전송 중…"
	case "VERIFYING":
		return "전송 완료 · 파일 무결성과 빌드 검사 중…"
	case "UPDATED":
		return "상대 기기에서 새 에이전트 버전 실행을 확인했습니다."
	case "RESTART_UNCONFIRMED":
		return "파일은 전달했지만 새 버전 실행을 확인하지 못했습니다. 상대의 관리 앱·에이전트를 확인하세요."
	case "SAME_VERSION":
		return "상대 기기는 보내려는 묶음과 같은 버전입니다. 업데이트가 필요하지 않습니다."
	case "PEER_NEWER":
		return "상대 기기가 보내려는 묶음보다 최신 세대입니다. 다운그레이드하지 않습니다."
	case "GENERATION_CONFLICT":
		return "같은 세대의 서로 다른 빌드입니다. 더 높은 세대로 만든 묶음이 필요합니다."
	case "PEER_BUSY":
		return "상대 기기가 다른 업데이트를 처리 중입니다. 완료 후 다시 시도하세요."
	case "BUNDLE_MISSING":
		return "업데이트 파일이 없습니다. ZIP 전체를 새 폴더에 풀어 실행하세요."
	case "HASH_MISMATCH":
		return "전송된 파일의 체크섬이 일치하지 않습니다. 적용하지 않았습니다."
	case "STORAGE_FAILED":
		return "상대 기기에 업데이트를 저장하지 못했습니다. 저장 공간과 폴더 권한을 확인하세요."
	case "TRANSFER_INTERRUPTED":
		return "파일 전송이 중단되었습니다. 네트워크를 확인한 뒤 다시 시도하세요."
	case "PREPARING":
		return "업데이트 묶음 검사 중…"
	case "QUEUED":
		return "상대 기기의 버전·업데이트 허용 확인 중…"
	case "RECEIVING":
		return "업데이트 수신·검증 중…"
	case "STAGED":
		return "업데이트 검증 완료 · 상대에게 수신 확인 중…"
	case "READY":
		return "업데이트 수신 완료 · 입력 연결을 정리하고 적용합니다."
	case "APPLYING":
		return "업데이트 적용 중…"
	case "DELIVERED":
		return "상대에게 파일 전달 완료 · 새 버전 실행 확인 중입니다. 확인 후 다시 연결합니다."
	case "PEER_APPLY_FAILED":
		return "상대가 업데이트 적용 실패를 보고했습니다. 자동 재연결을 중단했습니다. 상대 진단을 확인하세요."
	case "EXPIRED":
		return "업데이트 작업이 만료되었습니다. 상대 버전을 확인한 뒤 다시 보내세요."
	case "PEER_NOT_ALLOWED":
		return "상대 기기에서 이 기기의 업데이트를 먼저 허용하세요."
	case "INVALID_BUNDLE":
		return "업데이트 검사 실패 · ZIP 전체를 풀어 실행했는지 확인하세요."
	case "PEER_REJECTED":
		return "상대가 업데이트를 거부했지만 세부 이유를 제공하지 않았습니다. 구버전일 수 있으니 상대 진단을 확인하세요."
	case "TRANSFER_UNKNOWN":
		return "전송 결과 미확인 · 상대 기기의 버전을 확인하세요."
	case "RECEIVE_FAILED":
		return "업데이트 수신·검증 실패 · 기존 버전은 보존했습니다."
	case "APPLY_FAILED":
		return "업데이트 적용 실패 · 기존 버전을 다시 사용할 수 있습니다."
	case "REVOKED":
		return "업데이트 허용을 해제했습니다. 받은 묶음은 실행하지 않습니다."
	}
	return ""
}
