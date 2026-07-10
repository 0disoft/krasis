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

## 불변조건

- 활성 `ControlLease`는 기기 쌍에서 하나뿐이다.
- 새 `OperationalSession`은 이전 lease와 sequence를 재사용하지 않는다.
- `PressedState` 변경은 입력 주입보다 먼저 복구 가능한 형태로 기록되어야 한다.
- `FAULT`와 연결 종료의 안정 상태는 `LOCAL`이며, 그 전에 `ReleaseAll`을 시도한다.
- 알 수 없는 event, epoch 또는 sequence는 실행하지 않는다.
