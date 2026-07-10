# CLI 명령 계약

- 상태: Experimental
- 저장소 유형: cli-tool
- 실행 파일: `krasis-lab`
- 범위: Phase 0 Windows 입력 관찰

## 목적

`krasis-lab`은 제품 CLI가 아니라 Windows 입력 adapter를 선택하기 위한 폐기 가능한 실험 도구다. 명령과 JSON schema는 Phase 0 동안에만 호환성을 유지하며 정식 `krasis` CLI로 자동 승격되지 않는다.

## 명령

### `diagnose`

현재 프로세스와 Windows 입력 환경을 읽기 전용으로 진단한다.

- 사용법: `krasis-lab diagnose [--json]`
- 입력 hook을 설치하지 않는다.
- Windows/build, architecture, session, elevation, virtual screen과 API capability를 보고한다.
- 사용자 이름, 장치 이름, 창 제목, 네트워크 주소와 원시 입력을 출력하지 않는다.

### `capture`

저수준 keyboard/mouse hook을 설치하고 event 종류별 누적 개수만 보고한다.

- 사용법: `krasis-lab capture [--json] [--duration DURATION]`
- `--duration`이 없으면 `Ctrl+C`까지 실행한다.
- `--duration`은 Go duration 형식의 양수만 허용한다.
- key 값, scan code, 입력 문자열, pointer 좌표와 대상 창을 출력하지 않는다.
- 입력을 억제하거나 변경하거나 주입하지 않는다.
- hook callback은 atomic counter만 변경하고 출력은 별도 reporter가 수행한다.

## 출력 방식

- 기본 출력은 사람이 읽는 누적 snapshot이다.
- `--json`은 한 줄에 하나의 JSON object를 출력한다.
- `capture`의 기본 보고 간격은 현재 실험 구현에서 1초다. 이는 제품 성능 목표가 아니라 관찰용 sampling cadence다.

## 취소와 종료

- `Ctrl+C` 또는 `--duration` 만료 시 Windows message loop에 종료 message를 보내고 hook을 해제한다.
- 종료 시 마지막 누적 snapshot을 한 번 더 보고한다.
- 입력 억제가 없으므로 crash guard와 `ReleaseAll`은 아직 이 명령의 책임이 아니다.

## 지원 경계

- 정식 지원 대상은 아직 확정되지 않았다.
- 현재 실험 구현은 Windows에서 low-level hook을 사용한다.
- 다른 운영체제에서는 명시적인 unsupported 오류로 종료한다.
- 관리자 응용프로그램, UAC와 보안 데스크톱의 동작은 아직 검증되지 않았다.

## 변경 규칙

명령, flag, JSON field 또는 exit code를 바꾸면 이 문서와 `docs/cli/output-and-exit-codes.md`, 관련 test를 함께 변경한다.
