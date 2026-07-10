# 아키텍처

- 상태: Draft

## 경계

Krasis는 하나의 비공개 monorepo와 하나의 Go module로 시작한다. 상주 agent가 입력, 연결, 신원, 제어권과 복구를 소유한다. CLI와 Wails/Svelte UI는 동일한 로컬 application API를 사용하는 projection이며 실시간 입력 경로를 소유하지 않는다.

## 계층

- Domain: 세션, 제어권, epoch, 눌림 상태, 전환 상태 머신
- Application: 페어링, 연결, handoff, 해제, 복구 use case
- Adapter: Windows 입력, 네트워크, 신원 저장, 로컬 IPC, 진단
- Projection: tray, CLI, Wails/Svelte 설정 UI

Domain과 Application은 Win32, Wails, Svelte 및 구체적인 전송 라이브러리를 import하지 않는다.

## 런타임 흐름

물리 입력은 Windows adapter에서 정규화된 domain event가 된다. 활성 제어권이 없으면 로컬 처리를 유지한다. destination 준비와 유효한 제어권이 모두 확인된 경우에만 source 억제와 원격 전달을 시작한다. 어느 단계든 불확실하면 억제를 풀고 양쪽 눌림 상태를 해제한 뒤 `LOCAL`로 돌아간다.

## 결정과 가설의 경계

결정된 것은 실패 안전 방향, 계층 경계, UI 격리, 단일 활성 제어권이다. 저수준 hook과 Raw Input의 조합, TCP 연결 수, Protobuf, mTLS 세부 형식, crash guard 프로세스와 모든 정량 목표는 Phase 0 또는 ADR 검증 전 가설이다.
