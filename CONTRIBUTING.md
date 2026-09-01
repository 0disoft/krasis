# Contributing

- 상태: Active
- 책임: repository maintainers

## 먼저 읽을 문서

Krasis는 Windows 노트북 사이의 입력 제어를 다루므로 일반적인 데스크톱 앱보다 실패 비용이 크다.
작업 전에 `AGENTS.md`, `docs/product/02-spec.md`, `docs/product/01-roadmap.md`,
`docs/product/03-risk-register.md`, `docs/architecture/01-domain-model.md`,
`docs/architecture/02-runtime-flow.md`와 변경 영역의 ADR을 읽는다.
검증 이름과 최종 보고 형식은 `VALIDATION.md`, 리뷰 기준은 `CHECKLIST.md`가 소유한다.

## 기여 범위

하나의 PR은 하나의 문제와 rollback 경계를 가져야 한다. 상태 모델, wire contract, trust store,
입력 adapter, evidence schema를 바꾸면 구현과 테스트뿐 아니라 해당 source-of-truth 문서를 같은 PR에서
바꾼다. generated output, build artifact, cache, 내려받은 보고서와 로컬 실험 디렉터리는 source가 아니다.

현재 제품 경계 밖인 클립보드, 파일·화면 공유, 인터넷 relay, cloud account, telemetry upload,
비-Windows 지원과 자동 업데이트를 구현하려면 먼저 제품 명세와 ADR 승인이 필요하다.

## 안전 및 개인정보

원시 key code 시계열, 입력 문자열, pointer 좌표·궤적, window title, 사용자 이름, host name,
로컬 절대 경로, IP가 포함된 raw report와 private key를 issue, PR, log, fixture에 올리지 않는다.
Phase 0 결과는 저장소의 versioned aggregate schema와 고정 device alias만 사용한다.
실행하지 않은 물리 장비 검증을 완료했다고 쓰지 않는다.

제어권이나 입력 경로를 수정하는 PR은 destination 준비 전 suppression 금지, 합성 입력 재전달 금지,
unknown session·lease·epoch·sequence 실행 금지, 실패 뒤 `ReleaseAll`과 `LOCAL` 수렴을 유지해야 한다.

## 구현과 검증

개발 중에는 바뀐 package의 targeted test를 먼저 실행하고, merge 전에는 PR의 Windows CI를 한 번
완주한다. 문서만 바꾼 PR은 repository contract test와 CI로 충분하며 같은 전체 검사를 반복하지 않는다.

Go 변경의 기본 검증은 `gofmt`, `go test ./...`, repository vet policy와 Windows build다.
`internal/wininput`의 `unsafe` 경계는 일반 vet을 통째로 끄지 않고 CI에 기록된 package 한정 예외만 쓴다.
물리 hook, suppression, 절전·잠금, 장시간 입력은 자동 테스트가 대신할 수 없으며 관련 동작을 완료로
주장할 때만 runbook evidence를 요구한다.

## PR 계약

PR 본문은 변경 이유, source of truth, 위험, 실행한 검증, 건너뛴 검증과 이유, 남은 위험, rollback을
구체적으로 적는다. dependency 변경은 필요성, 현재 major 유지 여부, immutable pin, 보안·license·runtime
영향과 제거 비용을 적는다. 새 파일이나 명령은 help, JSON schema, exit code, privacy 경계를 함께 갱신한다.

## Merge 차단 조건

다음 변경은 merge하지 않는다.

- 현재 코드와 충돌하는 문서를 새 source of truth처럼 추가한다.
- failure를 성공이나 `LOCAL`로 가장하거나 cleanup 오류를 삼킨다.
- fixture와 report에 raw input, credential, 로컬 식별자 또는 절대 경로를 넣는다.
- generated output이나 물리 evidence를 사람이 실행한 것처럼 조작한다.
- 검증을 줄이면서 실패 비용과 대체 근거를 설명하지 않는다.
