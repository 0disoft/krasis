# 도메인 모델

- 상태: Draft

## 핵심 개체

- `DeviceIdentity`: 로컬 기기의 지속 신원
- `TrustedPeer`: 사용자가 승인한 상대 기기와 신뢰 상태
- `PairingSession`: 제한된 시간 동안 진행되는 상호 확인 절차
- `OperationalSession`: 현재 연결의 일회성 보안 및 순서 경계
- `ControlLease`: 어느 기기가 어느 방향의 입력을 소유하는지 나타내는 배타적 권한
- `InputStream`: epoch와 sequence로 구분되는 정규화 입력 흐름
- `PressedState`: 원격에 반영된 키와 버튼 집합
- `DisplayGeometry`: 활성 디스플레이와 경계 배치
- `PeerConfig`: 사용자 설정과 연결 정책

## 상태

최소 상태 집합은 `LOCAL`, `PENDING`, `OUTBOUND_ACTIVE`, `INBOUND_ACTIVE`, `REVOKING`, `FAULT`다. 세부 상태 추가는 모델 테스트에서 구분 필요성이 증명될 때만 한다.

`internal/control`의 transition table이 제어권 lifecycle의 실행 가능한 기준이다. Application과 adapter는 상태를 직접 대입하지 않고 event와 현재 snapshot을 이 table에 전달한다.

| 현재 상태 | Event | 다음 상태 | Effect |
| --- | --- | --- | --- |
| `LOCAL` | `OUTBOUND_INTENT_DETECTED` | `PENDING` | outbound lease 요청 |
| `PENDING` | `OUTBOUND_LEASE_GRANTED` | `OUTBOUND_ACTIVE` | outbound 입력 경로 시작 |
| `LOCAL` | `INBOUND_LEASE_GRANTED` | `INBOUND_ACTIVE` | inbound 입력 경로 시작 |
| `PENDING`, `OUTBOUND_ACTIVE`, `INBOUND_ACTIVE` | `REVOKE_REQUESTED` | `REVOKING` | 원격 제어 중지, lease 폐기, `ReleaseAll` |
| `PENDING`, `OUTBOUND_ACTIVE`, `INBOUND_ACTIVE`, `REVOKING` | `FAULT_DETECTED` | `FAULT` | 원격 제어 중지, lease 폐기, `ReleaseAll` |
| `FAULT` | `FAULT_DETECTED` | `FAULT` | `ReleaseAll` 재시도 |
| `REVOKING`, `FAULT` | `RELEASE_ALL_SUCCEEDED` | `LOCAL` | session/lease 식별자 폐기, 로컬 입력 복귀 |
| `REVOKING`, `FAULT` | `RELEASE_ALL_FAILED` | `FAULT` | `ReleaseAll` 재시도 |

`SessionID`는 연결마다 새로 생성되는 application 입력이고 `LeaseID`는 peer가 승인한 제어권 식별자다. `PENDING`에서 받은 grant의 session이 현재 session과 다르면 stale event로 거부한다. `LOCAL`로 돌아갈 때 두 식별자를 모두 지우므로 재연결이 이전 lease를 이어받을 수 없다.

현재 table은 순수 결정만 소유한다. effect 실행, transition history, durable compare-and-swap, event idempotency key와 네트워크 재조정은 Application 계층 책임이며 아직 구현되지 않았다. `ReleaseAll` 실패를 성공이나 로컬 복귀로 바꾸지 않는 것이 이 단계의 fail-safe 경계다.

## 입력 stream gate

`internal/control.InputStream`은 활성 연결의 `SessionID`, 일회성 `Epoch`와 정확히 다음에 받을 `Sequence`를 소유한다. 첫 sequence는 `1`이며 현재 session·epoch와 정확히 일치하는 다음 sequence만 승인한다.

- 이전 session 또는 epoch의 envelope는 stale 입력으로 거부한다.
- 이미 본 sequence는 duplicate로 거부하고 다시 실행하지 않는다.
- 다음 값보다 큰 sequence는 gap으로 거부하며 빠진 입력을 추측하거나 건너뛰지 않는다.
- sequence가 wraparound에 도달하면 stream을 계속 쓰지 않고 새 operational session이 필요하다.
- 거부된 envelope는 stream snapshot을 전진시키지 않는다.

이 gate는 message payload나 직렬화 형식을 정하지 않는다. transport adapter가 인증과 decode를 끝낸 뒤 Application이 envelope metadata를 gate에 넣고, 승인된 경우에만 입력 payload를 해석하고 실행해야 한다.

## Peer liveness

`PeerLiveness`는 현재 `SessionID`와 마지막 heartbeat를 로컬에서 관찰한 시각만 가진다. timeout 값은 domain 상수가 아니라 Application이 실측 또는 설정에서 전달하는 값이다. deadline에 도달하면 `FAULT_DETECTED` event를 만들며, 이전 session의 heartbeat와 시간이 뒤로 이동한 관찰은 liveness를 갱신하지 않는다.

peer가 보낸 wall clock을 신뢰하지 않고 로컬 수신 시각을 사용해야 한다. 실제 timer 소유권, heartbeat cadence와 timeout 값은 transport 및 성능 실험 전까지 `UNDECIDED`다.

## PressedState와 ReleaseAll

`PressedState`는 원격에 down을 반영하려는 keyboard key와 mouse button의 opaque normalized token을 메모리에 기록한다. token의 구체적인 Windows code mapping은 입력 adapter 검증 뒤 확정한다.

- down 주입 전에 token을 먼저 기록한다.
- 같은 token의 반복 down은 상태를 중복시키지 않는다.
- up 주입이 성공한 뒤에만 token을 제거한다.
- `ReleaseAll`은 최초 down의 역순으로 모든 token을 시도하며 중간 실패 뒤에도 나머지를 계속 해제한다.
- 성공한 token만 제거하고 실패한 token은 다음 retry를 위해 유지한다.
- 빈 상태의 `ReleaseAll`은 아무 입력도 만들지 않는다.

`PressedState`는 원시 입력 log가 아니다. 메모리상 복구 책임에만 사용하며 support bundle, 진단 출력과 일반 log에 token code를 복제하지 않는다. durable crash journal 형식과 암호화·수명 정책은 crash guard 설계 전까지 미구현이다.

Application의 `SessionRuntime`이 `ControlLease`, `InputStream`, `PeerLiveness`, `PressedState` 접근을 하나의 lock 아래 직렬화한다. 값 객체와 coordinator 함수를 runtime 밖의 여러 goroutine에서 직접 공유하는 사용은 지원하지 않는다. runtime이 호출하는 effect와 input port도 같은 runtime으로 재진입하면 안 된다.

## 불변조건

- 활성 `ControlLease`는 기기 쌍에서 하나뿐이다.
- 새 `OperationalSession`은 이전 lease와 sequence를 재사용하지 않는다.
- `PressedState` 변경은 입력 주입보다 먼저 복구 가능한 형태로 기록되어야 한다.
- `FAULT`와 연결 종료의 안정 상태는 `LOCAL`이며, 그 전에 `ReleaseAll`을 시도한다.
- 알 수 없는 event, epoch 또는 sequence는 실행하지 않는다.
