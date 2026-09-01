# Data Integrity

- 상태: Active
- persistent database: 없음
- durable product record: local trust store
- 실험용 local data: launcher config, credential directory, aggregate evidence

## Ownership

| data | owner | durability | source of truth |
| --- | --- | --- | --- |
| peer trust record와 revoke evidence | local trust store | persistent | ADR 0006, `krasis.trust-store/v1` |
| launcher role·alias·address·path 설정 | Phase 0 launcher | persistent local lab config | `krasis.launcher.config/v1` |
| certificate와 private key | user-selected protected credential directory | experiment lifetime | credential files 자체 |
| agent, guard와 probe report | report writer | user-managed evidence lifetime | versioned aggregate JSON |
| session, lease, epoch, sequence, liveness | `SessionRuntime` | process memory | current runtime snapshot |
| pressed-input recovery | agent와 crash guard memory | active session only | acknowledged token state |
| `build/`, `dist/`, `.cache/` | build process | disposable | source와 exact revision에서 재생성 |

runtime state, cache, package artifact와 copied report를 durable product source로 승격하지 않는다.

## Read Contract

trust, config와 report reader는 regular bounded file, exact schema, unknown-field rejection과 single JSON
value를 요구한다. malformed, oversized, unsupported schema, DACL mismatch, lock contention와 path escape를
빈 성공 상태나 default trust로 바꾸지 않는다.

trust revoke는 record를 삭제하지 않고 `revokedAt` evidence를 남긴다. revoked fingerprint를 import해
새 active record로 되살리지 않는다. operational reconnect는 이전 session, lease, epoch와 sequence를
재사용하지 않는다.

## Write Contract

persistent JSON은 target directory 안의 새 temporary file에 쓰고 flush와 close를 확인한 뒤 atomic
replacement 또는 no-replace promotion으로 승격한다. 기존 evidence output은 덮어쓰지 않는다.
실패하면 기존 valid file을 보존하고 partial target을 성공으로 노출하지 않는다.

Windows에서는 security-sensitive directory와 file의 protected DACL을 확인한다. report path는 parent
component의 symlink와 reparse point를 거부하고 promotion 직전에 다시 검사한다. 같은 사용자 권한의
path replacement 경쟁까지 완전히 막는 handle-relative writer는 아직 없다.

manifest와 checksum을 가진 probe kit는 archive를 신뢰하기 전에 expected file set, `BUILD.json`,
SHA-256 manifest와 exact source revision을 검증한다.

## Runtime Consistency

`SessionRuntime` 하나가 control snapshot, inbound/outbound stream, liveness와 `PressedState`를 같은 lock
아래 직렬화한다. adapter와 peer goroutine은 이를 우회해 mutable state를 공유하지 않는다.

input sequence를 승인한 뒤 injection 결과가 불확실하면 sequence를 재사용하거나 payload를 재생하지
않고 session을 fault 처리한다. pressed token은 down injection 전에 기록하고 successful release 뒤에만
제거한다. partial `ReleaseAll`은 성공한 token만 지우고 나머지를 보존해 retry한다. 모든 token이 해제되기
전에는 `LOCAL` 완료를 기록하지 않는다.

## Recovery

- trust/config write 실패: 이전 valid file 유지, 명시적 오류 반환
- corrupt 또는 unauthorized trust store: control 시작 거부, 자동 초기화 금지
- report write 실패: command 실패와 stable failure code, 기존 report overwrite 금지
- network/session failure: lease 폐기, `ReleaseAll`, safe `LOCAL` convergence
- agent crash: crash guard가 acknowledged token release 시도
- checksum 또는 package drift: launcher의 Pair와 connection 동작 차단
- cache/build corruption: disposable directory 삭제 후 source revision에서 재생성

backup, export, restore, schema migration UX와 secure deletion은 production data directory가 정해질 때
별도 설계한다. 지금은 이를 구현된 기능처럼 문서화하지 않는다.

## Privacy

raw key sequence, input text, pointer coordinate·trajectory, window title, username, hostname, network
identifier, absolute path, certificate body와 private key를 report나 ordinary log에 기록하지 않는다.
file reference가 필요하면 basename, fixed device alias, schema version, source revision과 aggregate count만
사용한다.

## 아직 결정하지 않은 integrity 계약

| 결정 | 책임 | blocker | 결정 trigger |
| --- | --- | --- | --- |
| production data root와 backup/export UX | repository maintainers | installer와 resident agent layout 미구현 | #13과 #16 |
| trust schema migration policy | repository maintainers | v2 requirement 없음 | first incompatible schema change |
| handle-relative Windows writer | repository maintainers | residual same-user race의 실제 release risk 미평가 | #16 threat review |
| retention과 secure deletion | repository maintainers | beta support workflow 미확정 | #18 beta policy |
