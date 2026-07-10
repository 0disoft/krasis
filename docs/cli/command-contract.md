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

### `selftest-inject`

`SendInput`과 low-level hook의 injected 분류가 같은 process에서 왕복하는지 확인한다.

- 사용법: `krasis-lab selftest-inject [--json]`
- selftest 전용 `dwExtraInfo` 표식을 붙인 상대 mouse move 두 건만 주입한다.
- 이동은 `+1/-1` 쌍이며 click, wheel과 keyboard event를 만들지 않는다.
- 두 event에는 `MOUSEEVENTF_MOVE_NOCOALESCE`를 사용한다.
- 실행 전후 cursor 위치가 다르면 강제로 되돌리지 않고 실패한다.
- low-level hook에서 injected flag와 selftest 표식이 모두 확인된 event만 관찰 성공으로 센다.
- 실행은 내부 deadline으로 제한되며 hook을 해제한 뒤 종료한다.
- `SendInput`은 UIPI 적용을 받으므로 높은 integrity level의 대상으로 주입할 수 있다는 뜻이 아니다.

이 명령은 injected 입력 식별을 검증한다. 아직 outbound 전송 경로가 없으므로 재전달 차단 전체를 증명하지 않는다.

## 출력 방식

- 기본 출력은 사람이 읽는 누적 snapshot이다.
- `--json`은 한 줄에 하나의 JSON object를 출력한다.
- `capture`의 기본 보고 간격은 현재 실험 구현에서 1초다. 이는 제품 성능 목표가 아니라 관찰용 sampling cadence다.
- `selftest-inject`의 짧은 deadline과 sampling cadence도 제품 지연 목표가 아니라 유한한 안전 실험 경계다.

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
