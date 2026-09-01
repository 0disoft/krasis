# ADR 0037: 제품 CLI와 local API projection

- 상태: Accepted
- 날짜: 2026-09-01
- 범위: 제품 CLI, local API client, config precedence, 출력과 exit code

## 결정

정식 제품 CLI는 `krasis.exe`다. `krasis-lab.exe`의 실험 명령을 승격하거나 재사용하지 않으며,
입력 hook·peer transport·trust 파일을 직접 소유하지 않는다. 모든 제품 동작은 현재 사용자 세션의
`krasis-agent.exe`가 게시한 authenticated loopback API를 통해 수행한다.

CLI는 status, diagnostics, topology, connect/disconnect, pairing, trust list/revoke와 safe stop을
제공한다. human output과 `--json` output은 분리하고 JSON은 local API의 versioned schema를 그대로
투영한다. 입력값은 API 호출 전에 제한적으로 검증하고 API response는 크기 제한, unknown field,
trailing value와 schema version을 strict하게 검사한다.

설정 우선순위는 `--control-dir`, `%LOCALAPPDATA%\Krasis\config.json`, 제품 기본
`%LOCALAPPDATA%\Krasis\control`, 그 directory 안에서 agent가 매 실행 게시하는 endpoint/token runtime
state 순이다. 환경 변수와 command line에 token을 넣지 않는다.

exit code는 success, internal, usage/config, policy denial, unavailable agent, authentication,
transport/protocol, unsafe/conflicting state를 구분한다. 오류 출력은 경로, token, endpoint, 원시 API
본문을 복제하지 않고 안정된 분류와 복구 행동만 보여준다.

## 결과

- CLI 종료는 agent나 활성 입력 세션의 수명을 소유하지 않는다.
- trust revoke는 CLI가 파일을 고치는 대신 agent의 atomic trust-store operation을 호출한다.
- topology는 현재 연결과 신뢰 peer를 투영하지만, 아직 저장된 배치가 없으면 `UNCONFIGURED`를 명시한다.
- shell completion은 안정된 beta 배포 경로가 생길 때 별도 생성하며 현재 binary와 함께 설치하지 않는다.
