# Testing Standard

- 상태: Active
- merge 기준: PR Windows CI와 변경에 필요한 수동 evidence
- 명령 source: `.github/workflows/ci.yml`, `VALIDATION.md`

## 원칙

Krasis 테스트의 목적은 모든 조합을 소진하는 것이 아니라 사용자를 입력에서 가두는 회귀, stale ownership,
순서 위반, credential 오인가와 evidence 손상을 빨리 막는 것이다. 구현 중에는 affected package의 작은
test를 실행하고 merge 전 한 번의 전체 CI로 끝낸다. 같은 commit에서 근거 없이 full suite를 반복하지 않는다.

## 자동 검증 계층

| 계층 | 소유 범위 | 예시 |
| --- | --- | --- |
| pure unit | transition, identity guard, sequence, pressed state, boundary detector | `internal/control` tests |
| application | serialized runtime, effect compensation, `ReleaseAll`, liveness | fake port를 쓰는 `internal/application` tests |
| session integration | framed transport, bidirectional lease, reconnect, stale input | `net.Pipe` 기반 `internal/peer` tests |
| persistence contract | strict schema, atomic replacement, DACL와 fail-closed recovery | trust, launcher, report writer tests |
| Windows adapter | build tags, Win32 mapping, lifecycle messages, injection certainty | Windows CI의 package tests와 builds |
| repository contract | source-of-truth 문서, `.ssealed` 등록과 approved content drift | `internal/repositorycontract` test |

시간, network와 OS callback을 다루는 test는 fake clock, bounded context와 deterministic port를 우선한다.
실제 sleep과 외부 network를 unit test의 성공 조건으로 쓰지 않는다.

## Merge-blocking CI

Windows CI는 다음을 순서대로 실행한다.

1. 모든 Go file의 `gofmt` drift 확인
2. `go test ./...`
3. 일반 package의 `go vet`
4. 명시된 `internal/wininput` package만 `-unsafeptr=false`로 vet
5. `krasis-lab.exe`와 `krasis-launcher.exe` build
6. probe-kit package, bundled checksum verification와 artifact upload

format, test, vet, build, package 또는 verification failure는 merge blocker다. validation을 건너뛰면 PR에
실행 불가 이유와 남은 위험을 적어야 하며 성공으로 표시하지 않는다.

## 물리 장비 evidence

low-level hook 관찰, selective suppression, high polling-rate mouse, touchpad gesture, sleep·lock·resume,
process kill, cable/network loss와 장시간 pair 품질은 자동 CI가 증명하지 못한다.
해당 동작의 issue acceptance를 닫을 때만 versioned runbook과 aggregate report를 요구한다.

물리 결과는 exact commit과 probe-kit checksum에 묶고 fixed device alias만 사용한다. raw event,
coordinate, credential, hostname, username, IP와 local path는 첨부하지 않는다. 장비가 없어서 실행하지
못한 검증은 `skipped`이며 합성 fixture로 대체 완료 처리하지 않는다.

## 회귀 추가 기준

bug fix에는 실패를 재현하는 가장 작은 deterministic test를 먼저 추가한다. 이미 상위 계층이 같은 계약을
직접 고정하면 중복 test를 늘리지 않는다. security 또는 data-loss regression은 원인 package와 recovery
boundary를 모두 고정한다. timing-only flaky retry, 무제한 goroutine, 무제한 fixture와 snapshot 남발은
허용하지 않는다.

## 문서 변경

source-of-truth 문서를 바꾸면 link, command, 상태, issue와 코드 symbol을 현재 tree에서 확인한다.
`internal/repositorycontract`는 generic scaffold 문구의 재도입, target document 누락, manifest
registration drift와 approved content hash drift를 `go test ./...`에서 차단한다.
