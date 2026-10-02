package productcli

func doctorHumanState(value string) string {
	if text, ok := map[string]string{
		"healthy": "연결과 진단 항목 정상", "agent_unavailable": "에이전트 응답 없음", "connection_not_ready": "연결 준비 필요", "protocol_mismatch": "프로토콜 불일치",
		"cli_agent_version_mismatch": "CLI와 실행 에이전트 버전 불일치", "ui_agent_version_mismatch": "앱과 실행 에이전트 버전 불일치", "installed_agent_version_mismatch": "설치 에이전트 버전 불일치",
		"checks_incomplete": "일부 항목 확인 불가", "launch_record_invalid": "최근 앱 실행 기록 검증 실패", "service_path_mismatch": "서비스 실행 설정 불일치", "input_installation_invalid": "입력 설치 검증 실패", "input_service_stopped": "입력 서비스 중지", "input_service_not_ready": "입력 서비스 준비 필요",
		"local_api_authentication_failed": "로컬 에이전트 인증 실패", "local_api_transport_failed": "로컬 에이전트 통신 실패", "local_api_protocol_failed": "로컬 에이전트 응답 형식 불일치", "config_invalid": "CLI 설정 확인 필요",
		"RUNNING": "실행 중", "CONNECTED": "연결됨", "CONNECTING": "연결 중", "RECONNECTING": "재연결 중", "DISCONNECTING": "연결 해제 중", "LOCAL": "로컬 입력", "COMPATIBLE": "호환됨", "INCOMPATIBLE": "호환되지 않음", "UNKNOWN": "확인 불가", "UNAVAILABLE": "응답 없음", "NOT_CHECKED": "미확인", "MATCHED": "일치", "MISMATCH": "불일치", "VERIFIED": "검증됨", "NOT_INSTALLED": "미설치", "NOT_RUNNING": "닫혀 있음", "STOPPED": "중지됨", "PENDING": "준비 중", "INVALID": "검증 실패", "NOT_FOUND": "없음", "RECORD_INVALID": "실행 기록 불일치",
	}[value]; ok {
		return text
	}
	return value
}

func doctorHumanCheck(value string) string {
	if text, ok := map[string]string{"agent": "에이전트", "connection": "연결", "protocol": "프로토콜", "cli_agent_version": "CLI/에이전트 버전", "ui_agent_version": "앱/에이전트 버전", "installed_agent_version": "설치/실행 에이전트 버전", "ui": "앱 창", "launch_record": "최근 앱 실행 기록", "installed_agent": "보호된 설치 에이전트", "service": "입력 서비스"}[value]; ok {
		return text
	}
	return value
}
