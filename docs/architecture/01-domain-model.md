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

## 불변조건

- 활성 `ControlLease`는 기기 쌍에서 하나뿐이다.
- 새 `OperationalSession`은 이전 lease와 sequence를 재사용하지 않는다.
- `PressedState` 변경은 입력 주입보다 먼저 복구 가능한 형태로 기록되어야 한다.
- `FAULT`와 연결 종료의 안정 상태는 `LOCAL`이며, 그 전에 `ReleaseAll`을 시도한다.
- 알 수 없는 event, epoch 또는 sequence는 실행하지 않는다.
