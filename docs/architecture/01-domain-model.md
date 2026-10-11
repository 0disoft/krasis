# 도메인 모델

- 상태: Active
- Phase 1 판정: `COMPLETE`

## 핵심 개체

- `DeviceIdentity`: 로컬 기기의 지속 신원
- `TrustedPeer`: 사용자가 승인한 상대 기기와 신뢰 상태
- `PairingSession`: 제한된 시간 동안 진행되는 상호 확인 절차
- `OperationalSession`: 현재 연결의 일회성 보안 및 순서 경계
- `ControlLease`: 어느 기기가 어느 방향의 입력을 소유하는지 나타내는 배타적 권한
- `InputStream`: session, epoch와 sequence로 구분되는 정규화 입력 흐름
- `PressedState`: 원격에 반영됐거나 반영 여부가 불확실한 키와 버튼 집합
- `DisplayGeometry`: 활성 디스플레이와 경계 배치
- `PeerConfig`: 사용자 설정과 연결 정책

`TrustedPeer`의 durable 표현은 `krasis.trust-store/v1` document의 record다. fingerprint와 인증서
역할은 identity, alias는 표시용 이름, `allowInboundControl`은 명시적 capability, `revokedAt`은
삭제하지 않는 폐기 증거다. 폐기 record를 같은 fingerprint import로 부활시키지 않는다.

## Phase 1 실행 기준

`internal/control/lease.go`의 `transitionRules`가 제어권 lifecycle의 실행 가능한 기준이다.
Application과 adapter는 상태를 직접 대입하지 않고 현재 snapshot과 event를 이 table에 전달한다.
아래 표는 그 규칙을 사람이 검토할 수 있게 옮긴 유일한 문서 계약이다. 규칙을 바꿀 때는 코드,
`TestTransitionTable`, 이 표를 한 PR에서 함께 바꿔야 한다.

최소 상태 집합은 `LOCAL`, `PENDING`, `INBOUND_PENDING`, `OUTBOUND_ACTIVE`,
`INBOUND_ACTIVE`, `REVOKING`, `FAULT`다.

| 현재 상태 | Event | 다음 상태 | 필수 조건 | Effect |
| --- | --- | --- | --- | --- |
| `LOCAL` | `OUTBOUND_INTENT_DETECTED` | `PENDING` | 새 session, lease 없음 | 없음 |
| `PENDING` | `OUTBOUND_LEASE_GRANTED` | `OUTBOUND_ACTIVE` | pending session 일치, 새 lease 존재 | `BEGIN_OUTBOUND` |
| `PENDING` | `OUTBOUND_LEASE_REJECTED` | `REVOKING` | pending session 일치, lease 없음 | `STOP_REMOTE_CONTROL`, `RELEASE_ALL` |
| `PENDING` | `OUTBOUND_LEASE_ABORTED` | `REVOKING` | pending session 일치, lease 존재 | `STOP_REMOTE_CONTROL`, `RELEASE_ALL` |
| `OUTBOUND_ACTIVE` | `OUTBOUND_LEASE_ABORTED` | `REVOKING` | 현재 session과 lease 모두 일치 | `STOP_REMOTE_CONTROL`, `RELEASE_ALL` |
| `LOCAL` | `INBOUND_LEASE_PREPARED` | `INBOUND_PENDING` | 새 session과 lease 존재 | 없음 |
| `INBOUND_PENDING` | `INBOUND_LEASE_COMMITTED` | `INBOUND_ACTIVE` | 준비된 session과 lease 모두 일치 | `BEGIN_INBOUND` |
| `INBOUND_PENDING` | `INBOUND_LEASE_ABORTED` | `REVOKING` | 준비된 session과 lease 모두 일치 | `STOP_REMOTE_CONTROL`, `RELEASE_ALL` |
| `PENDING` | `REVOKE_REQUESTED` | `REVOKING` | 현재 pending ownership | `STOP_REMOTE_CONTROL`, `REVOKE_LEASE`, `RELEASE_ALL` |
| `INBOUND_PENDING` | `REVOKE_REQUESTED` | `REVOKING` | 아직 commit되지 않은 inbound ownership | `STOP_REMOTE_CONTROL`, `RELEASE_ALL` |
| `OUTBOUND_ACTIVE`, `INBOUND_ACTIVE` | `REVOKE_REQUESTED` | `REVOKING` | 현재 active ownership | `STOP_REMOTE_CONTROL`, `REVOKE_LEASE`, `RELEASE_ALL` |
| `PENDING` | `FAULT_DETECTED` | `FAULT` | 현재 pending ownership | `STOP_REMOTE_CONTROL`, `REVOKE_LEASE`, `RELEASE_ALL` |
| `INBOUND_PENDING` | `FAULT_DETECTED` | `FAULT` | 아직 commit되지 않은 inbound ownership | `STOP_REMOTE_CONTROL`, `RELEASE_ALL` |
| `OUTBOUND_ACTIVE`, `INBOUND_ACTIVE` | `FAULT_DETECTED` | `FAULT` | 현재 active ownership | `STOP_REMOTE_CONTROL`, `REVOKE_LEASE`, `RELEASE_ALL` |
| `REVOKING` | `FAULT_DETECTED` | `FAULT` | cleanup 중 추가 fault | `STOP_REMOTE_CONTROL`, `REVOKE_LEASE`, `RELEASE_ALL` |
| `FAULT` | `FAULT_DETECTED` | `FAULT` | cleanup 재시도 | `RELEASE_ALL` |
| `REVOKING`, `FAULT` | `RELEASE_ALL_SUCCEEDED` | `LOCAL` | release 완료 | session과 lease 제거, `RESUME_LOCAL` |
| `REVOKING` | `RELEASE_ALL_FAILED` | `FAULT` | release 일부 또는 전부 실패 | `RELEASE_ALL` 재시도 |
| `FAULT` | `RELEASE_ALL_FAILED` | `FAULT` | release 재시도 실패 | `RELEASE_ALL` 재시도 |

`EventInboundLeaseGranted`는 table의 직접 전이가 아니다. `SessionRuntime.GrantInbound`가 제공하는
편의 경계이며 내부에서 `PrepareInbound`와 `CommitInbound`를 순서대로 실행한다. 준비 뒤 source
활성화나 wire write가 실패하면 `AbortInbound`가 `INBOUND_PENDING`을 cleanup한다. 이 2단계 경계를
건너뛰어 `LOCAL`에서 바로 `INBOUND_ACTIVE`로 바꾸는 구현은 허용하지 않는다.

`SessionID`는 연결마다 새로 생성되는 application 입력이고 `LeaseID`는 peer가 승인한 제어권
식별자다. outbound grant와 abort, inbound commit과 abort는 해당 단계가 요구하는 exact identity를
검사한다. `LOCAL`로 돌아갈 때 session과 lease를 모두 지우므로 재연결이 이전 lease를 이어받을 수
없다. 모든 성공 전이는 snapshot `Version`을 정확히 1 증가시키며, 거부된 event는 snapshot과 effect를
만들지 않는다.

outbound boundary intent의 `Edge`는 source에서 넘은 방향이다. `RIGHT`는 destination의 왼쪽 edge
진입, `LEFT`는 destination의 오른쪽 edge 진입을 뜻한다. edge는 `LeaseRequest`에만 실리며 lease
identity나 권한을 대신하지 않는다.

transition effect는 로컬 입력 억제, 활성화, lease registry 무효화와 `ReleaseAll` 같은 process 내부
작업만 뜻한다. wire request, grant, reject, abort, revoke와 input write는 전이 결과와 runtime snapshot을
받은 peer session orchestrator가 runtime lock 밖에서 수행한다.

## 입력 stream gate

`internal/control.InputStream`은 활성 연결의 `SessionID`, 일회성 `Epoch`와 정확히 다음에 받을
`Sequence`를 소유한다. 첫 sequence는 `1`이며 현재 session과 epoch에 정확히 일치하는 다음 값만
승인한다.

- 이전 session의 envelope는 `ErrStaleSession`으로 거부한다.
- 이전 epoch의 envelope는 `ErrStaleEpoch`로 거부한다.
- `Sequence < NextSequence`는 duplicate 또는 역순 도착으로 보고 `ErrDuplicateInput`으로 거부한다.
- `Sequence > NextSequence`는 유실 또는 앞선 도착으로 보고 `ErrInputSequenceGap`으로 거부한다.
- 거부된 envelope는 stream snapshot을 전진시키지 않고 payload를 실행하지 않는다.
- sequence가 wraparound 직전에 도달하면 `ErrStreamExhausted`로 session을 폐기한다.

Application runtime은 stale identity, gap, injection 실패처럼 stream의 안전성을 확정할 수 없는 오류를
현재 session의 fault로 승격하고 `ReleaseAll` 뒤 `LOCAL`로 수렴한다. 이미 정상 local return을 마친
exact retired session의 연속 input만 주입 없이 drain한다. 이 예외는 늦게 도착한 이미 전송된 frame이
새 session을 fault 내는 일을 막기 위한 것이며, 다른 session이나 epoch에는 적용하지 않는다.

outbound source는 `SessionRuntime.BeginOutbound`에서 session과 epoch를 함께 만들고 `PENDING`부터
`OutboundStream`을 소유한다. 같은 session과 epoch를 포함한 grant만 `OUTBOUND_ACTIVE`로 전환할 수
있으며 sequence를 `1`부터 단조 증가시킨다. 예약한 sequence의 전송 결과가 불확실하면 값을 재사용하지
않고 session 전체를 폐기한다.

재연결은 이전 runtime이 `LOCAL`에 도달한 뒤 새 `SessionID`, 새 `Epoch`, sequence `1`로 시작한다.
이전 session의 envelope, heartbeat, return intent와 revoke는 새 ownership을 되살리거나 취소할 수 없다.

## 정규화 입력 action

`InputAction`은 keyboard key down/up, mouse button down/up, relative pointer move와 horizontal/vertical
wheel delta를 표현한다. key와 button은 opaque code만 가지며 delta를 가질 수 없고, move와 wheel은
code 없이 하나 이상의 non-zero delta가 필요하다.

Windows keyboard code는 low-level hook의 non-zero scan code를 하위 16비트에 넣고 bit 16 하나로
extended-key 여부를 표시한다. 나머지 상위 비트는 금지한다. destination은 virtual-key로 다시
해석하지 않고 `KEYEVENTF_SCANCODE`와 필요할 때 `KEYEVENTF_EXTENDEDKEY`를 사용해 주입한다. mouse
button code는 `1=left`, `2=right`, `3=middle`, `4=X1`, `5=X2`만 허용한다. 이 mapping은 transport
payload와 `PressedState`가 공유하는 실행 계약이다.

Application은 action을 검증한 뒤 현재 session, epoch와 sequence를 승인하고 같은 `SessionRuntime`
직렬 경계 안에서 adapter를 호출한다. adapter 호출이 실패하면 승인한 sequence를 재시도하지 않고
해당 session을 fault cleanup한다.

## Peer liveness

`PeerLiveness`는 현재 `SessionID`와 마지막으로 승인한 heartbeat 또는 정상 처리한 현재 inbound 입력의 로컬 관찰 시각을 가진다(ADR 0071). timeout 값은
domain 상수가 아니라 Application이 실측 또는 설정에서 전달하는 값이다. deadline에 도달하면
`FAULT_DETECTED` event를 만들며, 이전 session의 heartbeat와 시간이 뒤로 이동한 관찰은 liveness를
갱신하지 않는다.

peer가 보낸 wall clock을 신뢰하지 않고 로컬 수신 시각을 사용한다. 실제 timer 소유권, heartbeat
cadence와 제품 timeout 값은 transport 및 성능 실험에서 결정한다.

## PressedState와 ReleaseAll

`PressedState`는 원격에 down을 반영하려는 keyboard key와 mouse button의 normalized token을 메모리에
기록한다. keyboard token은 위 scan-code encoding을, mouse token은 닫힌 button code 집합을 사용한다.

- down 주입 전에 token을 먼저 기록한다.
- 같은 token의 반복 down은 상태를 중복시키지 않는다.
- up 주입이 성공한 뒤에만 token을 제거한다.
- 같은 token의 반복 up은 아무 상태도 바꾸지 않는다.
- `ReleaseAll`은 최초 down의 역순으로 모든 token을 시도하며 중간 실패 뒤에도 나머지를 계속 해제한다.
- 성공한 token만 제거하고 실패한 token은 다음 retry를 위해 유지한다.
- 빈 상태의 `ReleaseAll`은 아무 입력도 만들지 않는다.
- release가 하나라도 남으면 `LOCAL` 완료로 가장하지 않고 `FAULT`에서 재시도한다.

`PressedState`는 원시 입력 log가 아니다. 메모리상 복구 책임에만 사용하며 support bundle, 진단 출력과
일반 log에 token code를 복제하지 않는다. Phase 0 `agent-session`은 별도 crash guard process가 같은
normalized token set만 메모리에 복제한다. agent는 down 주입 전에 `PREPARE_PRESS` ACK를 받고,
성공한 up 뒤 `RELEASED` ACK를 받는다. pipe EOF를 포함한 guard 종료는 남은 token의 역순
`ReleaseAll`을 먼저 수행한다. durable file journal과 제품 설치 수명 정책은 도입하지 않았다.

## 화면 경계 intent

`BoundaryDetector`는 한 peer 배치에 대해 `LEFT` 또는 `RIGHT` edge 하나를 감시한다. virtual screen의
left 좌표와 width, activation gap, rearm distance를 명시적으로 받으며 음수 virtual-screen 좌표도
지원한다. pointer가 edge 방향으로 이동하며 activation band에 도달하고 physical key/button pressed
set이 비어 있을 때만 intent를 한 번 만든다. intent 뒤에는 안쪽 rearm distance까지 이동해야 다시
armed 상태가 된다.

MVP에서는 key나 mouse button을 누른 채 화면을 넘는 전환을 허용하지 않는다. edge intent 자체는
lease나 suppression을 시작하지 않으며 application orchestrator가 peer 준비와 grant를 확인한 뒤에만
source session을 활성화한다.

Application의 `SessionRuntime`이 `ControlLease`, `InputStream`, `OutboundStream`, `PeerLiveness`,
`PressedState` 접근을 하나의 lock 아래 직렬화한다. 값 객체와 coordinator 함수를 runtime 밖의 여러
goroutine에서 직접 공유하는 사용은 지원하지 않는다. runtime이 호출하는 effect와 input port도 같은
runtime으로 재진입하면 안 된다.

## 결정적 검증 근거

Phase 1 완료 판정은 Win32 hook, `SendInput`, 실제 TCP, TLS와 물리 장비에 의존하지 않는 아래 테스트를
직접 근거로 삼는다.

| 계약 | 구현 | 직접 검증 |
| --- | --- | --- |
| 모든 제어권 전이와 effect | `internal/control/lease.go` | `TestTransitionTable`이 규칙 수와 모든 전이를 1:1로 고정한다. |
| stale session과 lease 거부 | `control.Apply` guard | `TestOutboundGrantRejectsStaleSession`, `TestActiveLeaseEventsRejectStaleLease` |
| 잘못된 전이는 상태와 effect를 만들지 않음 | `control.Apply` | `TestInvalidTransitionProducesNoResult`, `TestInvalidSnapshotCannotTransition` |
| 새 session이 이전 lease를 재사용하지 않음 | release 성공 시 identity 제거 | `TestNewSessionDoesNotReuseReleasedLease` |
| exact next sequence만 승인 | `InputStream.Accept` | `TestInputStreamAcceptsOnlyExactNextSequence` |
| stale, duplicate, 역순, 유실과 gap은 전진 없이 거부 | `InputStream.Accept` | `TestInputStreamRejectsUnsafeEnvelopesWithoutAdvancing` |
| 재연결은 새 session과 epoch를 요구 | `NewInputStream` | `TestInputStreamRequiresNewEpochAfterReconnect` |
| sequence wraparound 차단 | `InputStream.Accept` | `TestInputStreamRefusesSequenceWraparound` |
| 반복 down/up의 idempotency와 역순 release plan | `PressedState` | `TestPressedStateMakesRepeatedDownAndUpIdempotent`, `TestPressedStatePlansReversePressOrder` |
| release 실패 token만 보존하고 재시도 | `application.ReleaseAll` | `TestReleaseAllContinuesAfterFailureAndRetainsOnlyFailedTokens` |
| 완전 release와 빈 release의 안정성 | `application.ReleaseAll` | `TestReleaseAllReleasesInReverseOrderAndClearsState`, `TestReleaseAllIsIdempotentWhenNothingIsPressed` |
| gap, stale heartbeat와 주입 실패 뒤 fail-safe `LOCAL` | `SessionRuntime` | `TestSessionRuntimeSequenceGapFailsSafe`, `TestSessionRuntimeStaleHeartbeatFailsSafe`, `TestSessionRuntimeInjectionFailureReleasesRecordedPress` |
| exact revoke와 local return 뒤 ownership 제거 | `SessionRuntime` | `TestSessionRuntimeAcceptsExactPeerRevoke`, `TestSessionRuntimeRevokesExactLocalReturn` |
| 늦은 retired input은 주입 없이 drain, 다른 identity는 거부 | `SessionRuntime` | `TestSessionRuntimeDrainsOrderedInputAfterLocalReturnWithoutInjection` |
| stale return은 현재 lease를 파괴하지 않음 | `SessionRuntime` | `TestSessionRuntimeIgnoresStaleLocalReturnWithoutDestroyingCurrentLease` |

`TestPeerSessionsReconnectWithNewSessionAndEpoch`와
`TestPeerSessionsRejectStaleInputAfterReconnect`는 실제 framed stream을 사용하는 보조 통합 근거다.
이 두 테스트는 위 순수 모델 테스트를 대신하지 않으며, session orchestration이 같은 계약을 지키는지만
확인한다.

## 불변조건

- 활성 `ControlLease`는 한 runtime에서 하나뿐이다.
- 새 `OperationalSession`은 이전 session, epoch, lease와 sequence를 재사용하지 않는다.
- `INBOUND_ACTIVE`는 반드시 prepare와 commit을 모두 통과한 뒤에만 도달한다.
- `PressedState` 변경은 입력 주입보다 먼저 복구 가능한 형태로 기록한다.
- revoke와 fault의 완료 상태는 `LOCAL`이며, 그 전에 `ReleaseAll`을 실행한다.
- release가 남은 상태를 `LOCAL`로 기록하지 않는다.
- 알 수 없는 event, session, lease, epoch 또는 sequence는 실행하지 않는다.
