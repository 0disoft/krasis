# CLI Tool

- Status: Active
- Executables: `krasis.exe`, `krasis-lab.exe`
- Owner: repository maintainers
- ADR: `docs/adr/0037-product-cli-and-local-api-projection.md`

`krasis.exe`는 실행 중인 resident agent의 authenticated management client다. status, diagnostics,
topology, connect/disconnect, pairing, trust inspection/revocation과 safe stop을 제공한다. hook, peer
transport, trust 파일 mutation이나 session lifecycle을 직접 소유하지 않는다.

`krasis-lab.exe`는 Phase 0 증거 수집 도구다. 제품 config를 읽지 않고 제품 CLI 명령이나 exit code와
호환성을 약속하지 않는다.

현재 제품 runtime floor는 limited-beta 정책과 동일한 Windows 11 amd64다. CLI와 agent는 같은 설치
revision의 `/v1` schema를 사용해야 하며, 예상하지 않은 schema version은 exit `6`으로 거부한다. 다른
Windows version·architecture는 단순 compile 성공만으로 지원하지 않는다.

계약 문서:

- 명령과 argument: `command-contract.md`
- 설정 우선순위: `configuration.md`
- human/JSON 출력과 exit code: `output-and-exit-codes.md`
- completion 정책: `shell-completion.md`

private key, certificate body, token, endpoint, username, absolute control path와 원시 API 오류는 어느 출력
모드에도 포함하지 않는다. fingerprint와 pairing confirmation code는 해당 사용자가 명시적으로 요청한
trust/pairing command에서만 표시한다.
