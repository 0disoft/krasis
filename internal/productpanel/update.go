package productpanel

func UpdateNotice(state string) string {
	switch state {
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
		return "업데이트 전송 대기 · 상대 기기의 업데이트 허용이 필요합니다."
	case "RECEIVING":
		return "업데이트 수신·검증 중…"
	case "STAGED":
		return "업데이트 검증 완료 · 상대에게 수신 확인 중…"
	case "READY":
		return "업데이트 수신 완료 · 입력 연결을 정리하고 적용합니다."
	case "APPLYING":
		return "업데이트 적용 중…"
	case "DELIVERED":
		return "상대에게 업데이트 전달 완료 · 적용 후 연결을 다시 준비합니다."
	case "EXPIRED":
		return "전송 대기 만료 · 상대의 업데이트 허용·버전·방화벽을 확인하세요."
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
