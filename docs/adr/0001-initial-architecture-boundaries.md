# ADR 0001: 상주 agent와 projection 분리

- 상태: Accepted
- 책임자: cherr

## 맥락

Krasis의 입력 처리는 GUI 수명, 렌더러 정지 또는 프론트엔드 도구 변경과 분리되어야 한다. 잘못된 종료는 단순 UI 오류가 아니라 사용자가 입력을 잃는 안전 문제다.

## 결정

- 사용자 세션에서 동작하는 상주 Go agent가 입력, 연결, 신원, 제어권과 복구를 소유한다.
- CLI, tray와 Wails v3/Svelte UI는 agent의 로컬 API를 사용하는 projection이다.
- Domain과 Application은 Win32와 UI framework에 의존하지 않는다.
- 초기 저장소는 비공개 monorepo와 단일 Go module로 유지한다.
- Windows service는 MVP에 두지 않는다.

## 결과

UI를 교체하거나 닫아도 입력 안전 계약은 유지된다. 대신 agent, local API와 projection 사이의 버전 계약과 단일 인스턴스 lifecycle을 명시적으로 관리해야 한다.

## 보류

Phase 0 lab의 crash guard process와 IPC 순서는 ADR 0004에서 결정했다. 정식 제품의 process
수, 설치 범위, single-instance와 updater 경계는 여전히 Phase 0 이후 결정한다.
