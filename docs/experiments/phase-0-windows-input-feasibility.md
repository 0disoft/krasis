# Phase 0: Windows 입력 안전성 검증

- 상태: In Progress
- 책임자: cherr
- 제품 범위 기준: `docs/product/02-spec.md`
- 아키텍처 기준: `ARCHITECTURE.md`
- 결과 기록: 이 문서, `docs/experiments/results/`의 redacted 실행 기록과 후속 ADR

## 목적

정식 Krasis를 만들기 전에 실제 대상 노트북에서 다음 질문에 답한다.

1. Go 프로세스가 Windows 물리 입력을 빠뜨리거나 중복하지 않고 관찰할 수 있는가?
2. 로컬 입력을 필요한 동안만 억제하고 어떤 실패에서도 다시 풀 수 있는가?
3. 원격 입력에 해당하는 합성 입력을 주입하고 다시 전달되는 루프를 차단할 수 있는가?
4. 프로세스 종료, 연결 단절, 절전과 잠금 뒤 키와 버튼을 눌린 상태로 남기지 않을 수 있는가?
5. 순수 Go adapter로 충분한가, 작은 native 모듈 또는 별도 crash guard가 필요한가?

이 단계의 산출물은 제품 코드가 아니라 입력 adapter를 선택하거나 프로젝트를 중단할 수 있는 재현 가능한 증거다.

## 범위

### 포함

- 키보드 key down/up 관찰
- 마우스 이동, 휠, 버튼 down/up 관찰
- 물리 입력과 합성 입력 구분
- 합성 입력 주입
- 선택적 로컬 입력 억제와 자동 해제
- 단일 활성 디스플레이의 좌우 경계 감지
- 프로세스 비정상 종료, 잠금, 절전과 사용자 세션 변화
- 일반 권한과 관리자 권한 창 사이의 실제 제한
- 내장 터치패드와 사용 중인 외장 마우스

### 제외

- peer discovery, 인터넷 또는 LAN 전송
- 페어링, 인증서와 암호화 protocol
- Wails, Svelte와 tray UI
- 설치 프로그램과 자동 업데이트
- 다중 모니터, 세 대 이상, 클립보드와 파일 공유
- 로그인 화면과 Windows 보안 데스크톱 지원
- kernel driver 개발

## 확인된 환경과 미확정 환경

사용자에게서 확인된 사실은 Windows 노트북이 집에 두 대, 스터디카페에 두 대 있다는 것이다. 다음 값은 장비 조사 전까지 `UNDECIDED`다.

- Windows edition, version과 build
- CPU architecture
- 관리자 권한 사용 가능 여부
- DPI와 배율
- 터치패드 제조사와 driver
- 외장 마우스 model과 polling rate
- Modern Standby 지원 여부
- 백신 또는 EDR
- 공용 Wi-Fi의 client isolation

## 장비 원장

실제 값이 확인되기 전에는 추측해서 채우지 않는다.

| ID | 장소 | Windows/build | CPU | DPI/배율 | 입력 장치 | polling rate | Standby | 보안 제품 | 관리자 권한 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| H-A | 집 | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED |
| H-B | 집 | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED |
| S-A | 스터디카페 | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED |
| S-B | 스터디카페 | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED | UNDECIDED |

## 안전 전제

입력 억제 실험 빌드는 아래 조건을 모두 충족하기 전에는 실행하지 않는다.

- 억제는 명시적으로 시작한 짧고 제한된 실험 구간에서만 활성화된다.
- watchdog이 실험 프로세스와 독립적으로 억제를 자동 해제한다.
- 실험 프로세스 종료와 예외 처리 경로가 모두 억제를 해제한다.
- 긴급 해제 동작은 실험 대상 입력 경로 하나에만 의존하지 않는다.
- 다른 노트북이나 별도 입력 장치처럼 실험 프로세스를 종료할 독립적인 복구 수단이 준비되어 있다.
- 첫 억제 실험은 중요한 작업과 저장되지 않은 문서가 없는 장비에서 수행한다.
- 두 노트북에서 동시에 억제 실험을 시작하지 않는다.

watchdog과 독립 복구 수단이 없는 구현은 기능이 동작하더라도 Phase 0 후보에서 탈락한다.

## 개인정보와 증거 수집 원칙

- 실제 key 값, 입력 문자열, 마우스 절대 좌표 궤적을 파일에 저장하지 않는다.
- 증거에는 event 종류, 물리/합성 분류, sequence, 상대 시각, 처리 결과와 오류 code만 기록한다.
- 창 제목, 실행 중인 프로그램 이름, 사용자 이름과 네트워크 식별자는 기본 수집 대상이 아니다.
- 화면 녹화가 필요하면 테스트용 빈 바탕화면과 테스트용 문서만 사용한다.
- support bundle을 만들기 전 원시 입력이 포함되지 않았는지 별도 검사한다.

## 후보 구현 경계

실험용 CLI의 가칭은 `krasis-lab`이다. 명령 이름은 제품 CLI 계약이 아니며 Phase 0 뒤 삭제하거나 변경할 수 있다.

| 실험 경계 | 역할 |
| --- | --- |
| `capture` | 입력 event 관찰과 물리/합성 분류 |
| `inject` | 제한된 테스트 입력 주입과 정리 |
| `suppress` | 자동 해제가 포함된 선택적 억제 |
| `edge` | 좌우 경계 의도 관찰 |
| `recover` | 눌림 상태 정리와 `ReleaseAll` 검증 |
| `diagnose` | 환경과 adapter capability 출력 |
| `probe` | 한 장비의 진단, bounded capture와 주입 selftest를 독립 JSON 증거로 묶음 |
| `stress-selftest` | 같은 process에서 hook session과 tagged 주입 왕복을 bounded window 동안 반복 |
| `guided-probe` | keyboard, pointer, wheel/buttons를 단계별 aggregate evidence로 확인 |
| `probe-check` | 장비별 basic probe JSON의 schema와 비교 가능성 검증 |
| `pointer-shadow` | Raw Input relative packet과 legacy hook edge delta 소실을 aggregate evidence로 비교 |
| `wheel-surface` | 한 장비에서 hook·Raw Input wheel 표면과 tagged pointer-move hook sentinel을 분리 계측 |
| `touchpad-surface` | Windows 11 touchpad-capable foreground window에서 `PT_TOUCHPAD` pointer·legacy wheel 표면을 aggregate 계측 |

각 경계는 사람에게 읽히는 결과와 redacted machine-readable 결과를 같은 내부 결과 model에서 만들어야 한다. 구체적인 flag와 exit code는 구현 직전에 CLI 계약으로 확정한다.

## 현재 구현 상태

- `diagnose`: 구현 및 현재 개발 호스트 smoke 완료
- `capture`: low-level keyboard/mouse hook, aggregate counter, bounded duration과 정상 hook 해제 구현
- `selftest-inject`: tagged relative mouse move 왕복의 전송, hook 관찰과 cursor 복귀 검증 구현
- `stress-selftest`: physical input 간섭 attempt를 분리하면서 첫 clean defect에서 중단하는 반복 검증 구현
- `probe`: 장비 별칭, build 식별자, 진단, aggregate capture와 selftest를 교체 불가 JSON file로 저장하도록 구현
- `guided-probe`: keyboard, pointer, wheel/buttons 단계별 누락 code와 aggregate snapshot 저장 구현
- `probe-check`: unknown field, trailing JSON, size, schema, build와 duration 비교 검증 구현
- `pointer-shadow`: Raw Input과 low-level hook을 동시에 관찰하되 suppression·주입·network 전달 없이 edge-clipping signature만 aggregate JSON으로 저장하는 lab-only characterization 구현
- `wheel-surface`: hook physical/synthesized wheel과 Raw Input wheel flag·signed delta를 집계하고 시작·종료의 tagged 상대 pointer move `+1/-1` pair로 hook health를 확인하는 단일 장비 진단 구현
- `touchpad-surface`: visible foreground window에 Windows 11 touchpad capability를 등록하고 `PT_TOUCHPAD` pointer와 legacy wheel을 내용 없이 aggregate로 구분하는 단일 장비 진단 구현
- 장비 실행 kit: Windows binary, 수집 script, runbook, build identity와 SHA-256 package 구현
- 일반 `inject`, `suppress`, `edge`, `recover`: 미구현
- production-shaped hybrid mouse source: Raw Input relative move·button, zero-device 허용,
  current packet 표준 read와 64KiB buffered drain, active touchpad-capable window wheel owner와 focus 복원까지 구현;
  실제 S-A/S-B `agent-session/v13` touchpad wheel 재검증 통과

기존 `agent-session` wheel 0은 Raw Input wheel 부재 증거가 아니다. 당시 Raw mapper가
`RI_MOUSE_WHEEL`과 `RI_MOUSE_HWHEEL`을 의도적으로 무시했으므로 ADR 0013, 0017과 0018의 해당
증거 결론은 ADR 0019가 대체한다. 이후 standalone `wheel-surface`에서 hook과 Raw Input wheel이
`77/77`인 `HOOK_AND_RAW`가 관찰돼 Raw 부재 가설은 반박됐다. 그래도 두 값은 같은 OS event를
correlate하지 않는 aggregate라 split suppression·send ownership의 근거가 아니다. ADR 0021은 v12까지
hook-owned payload를 유지했고 ADR 0023은 gram13 touchpad surface 결과를 근거로 v13 active outbound
payload owner를 touchpad-capable window로 대체한다. Raw wheel은 계속 진단 evidence로만 센다.

현재 진단 계약은 `krasis.lab.agent-session/v13`, strict checker
`krasis.lab.agent-session-check/v10`, Phase 0 aggregate `krasis.lab.phase0-check/v10`이다.
`sourcePipeline.wheel`은 기존 hook/mapping/queue/send counter에 `rawObserved`,
`activeHookMoves`, `activeHookButtons`, `windowObserved`와 capture/focus lifecycle을 추가한다. 이 값들은 active source session의 독립 aggregate이며
같은 event나 exact equality를 증명하지 않는다. `rawObserved>0 && sent==0`은
`raw_wheel_observed_without_forwarding`으로 report를 실패시키는 다음 안전 gate다.

2026-07-16 gram13 `agent-session/v12` 수동 재현에서는 active source pointer move `290`건과 button
`4`건이 관찰되고 HP15에서 move/click/drag가 동작했지만 hook·합성·Raw wheel과 destination wheel은
모두 `0`이었다. 두 손가락 vertical/horizontal scroll은 gram13 local window에서 동작했다. 따라서
network나 전체 hook thread 실패로 보지 않고 ADR 0022의 `touchpad-surface`로 Windows 11
Precision Touchpad pointer 표면을 확인했다. 해당 실행은 `PT_TOUCHPAD` `1850`건, legacy vertical
`396`건과 horizontal `297`건의 `POINTER_AND_LEGACY`를 남겼고 type query 실패와 foreground 이탈은
`0`이었다. ADR 0023의 v13은 active outbound에서 이 system legacy 변환을 window-owned queue ingress로
사용한다.

2026-07-16 S-B source와 S-A destination의 `agent-session/v13` 재검증에서 사용자가 destination
scroll 동작을 확인했다. source window observation, queue accepted와 sent는 각각 `601`이었고 destination
wheel도 `601`이었다. capture activation/deactivation은 `7/7`, focus restore failure와 destination
injection failure, 양쪽 crash guard remaining은 모두 `0`이었다. 세부 redacted 결과와 남은 gate는
`docs/experiments/results/2026-07-16-study-pair-v13.md`가 소유한다. 외장 mouse 중복, 세로/가로 분리,
강제 foreground 이탈과 crash/disconnect/lifecycle 장애는 아직 장비 검증 대기다.

sentinel은 wheel이 아니라 `MOUSEEVENTF_MOVE|MOUSEEVENTF_MOVE_NOCOALESCE` tagged 상대 이동이라
Raw wheel evidence를 만들지 않는다. 정상 hook은 네 `WM_MOUSEMOVE`를 삼키지만 hook 누락 시 cursor가
순간 이동할 수 있고 pair 순합 0도 완전 무해를 보장하지 않는다. pass에는 sentinel health와 실제
non-sentinel move 또는 button interaction이 필요하며, 이 조건을 만족한 `NEITHER`는 유효한 진단 결과다.

현재 개발 호스트에서 확인한 값은 Windows `10.0` build `26200`, `amd64`, 일반 권한 process, 단일 monitor다. 이는 H-A/H-B/S-A/S-B 중 어느 장비인지 확인되지 않았으므로 장비 원장에는 반영하지 않았다. 같은 smoke에서 low-level keyboard/mouse hook, Raw Input과 `SendInput` API 존재를 확인했다.

1초 bounded `capture` smoke는 두 hook의 설치, 실제 물리 mouse move와 wheel callback, aggregate count 증가와 자동 hook 해제를 확인했다. 출력에는 좌표나 raw key가 포함되지 않았다. 1초는 제품 성능 목표가 아니라 자동 검증을 유한하게 끝내기 위한 실행 경계다. keyboard callback과 injected event 분류는 unit test로만 검증됐으며 실제 장비 입력 증거는 아직 필요하다.

`selftest-inject`는 zero-delta event를 사용하지 않고 `+1/-1` 상대 이동 두 건에 selftest 표식을 붙인다. 과거 같은 개발 호스트에서 반복 실행이 통과했지만, 현재 세션의 최신 재검증에서는 `SendInput`이 두 event를 수락하고 cursor도 원위치였음에도 low-level hook의 전체 injected move와 표식 event가 모두 `0`이었다. batch 상대 이동, 순차 상대 이동과 absolute 이동을 대조해도 같은 결과였으므로 현재 입력 desktop 또는 Windows 정책 차이를 포함한 환경 원인을 아직 배제할 수 없다. 따라서 현재 개발 호스트의 selftest와 stress 결과는 `INCONCLUSIVE`이며 네 대의 대상 노트북에서 새 artifact로 다시 검증해야 한다.

같은 process에서 bounded `capture` 직후 `selftest-inject`를 실행한 첫 `probe` 반복에서는 `SendInput`이 두 event를 수락했지만 두 번째 hook session이 이를 관찰하지 못했다. 각 hook session은 전용 Windows message-loop thread를 새로 만들고 종료 시 그 thread를 폐기해 이전 session의 message queue를 재사용하지 않는다. 종료 message를 게시하는 goroutine도 hook 정리 전에 합류한다. probe 자체는 `diagnose`, selftest, physical capture 순서로 실행한다. 대상 장비별 반복 검증은 여전히 필요하다.

네 대를 같은 시각에 실행하는 것은 요구사항이 아니다. 집에서는 `H-A`, `H-B`, 스터디카페에서는 `S-A`, `S-B` report를 각각 수집하고 나중에 같은 build revision과 실험 조건인지 비교한다. 한 장소의 두 장비가 동시에 필요한 실험은 peer 연결과 장애 경로를 다루는 후속 단계이며, 현재 `probe`는 한 번에 한 장비만 검사한다.

장비별 기초 수집 예시는 `krasis-lab probe --device H-A --duration 30s --output H-A.json` 형태다. output parent는 미리 만들고 장비마다 새 file name을 사용한다. `30s`는 권장 성능 기준이 아니라 손으로 pointer, wheel, button과 keyboard를 한 번씩 조작할 시간을 주는 예시일 뿐이다.

실제 artifact 실행 절차와 checksum 확인은 `docs/experiments/device-probe-runbook.md`가 소유한다.

## 공통 기록 항목

모든 실험은 다음 항목을 남긴다.

- 실험 ID와 장비 ID
- git commit 또는 실험 빌드 식별자
- Windows/build와 권한 수준
- 입력 장치와 display 배율
- 준비 상태와 복구 수단
- 재현 절차
- 관찰 결과
- 예상과 실제의 차이
- 안전 불변조건 위반 여부
- log와 화면 녹화의 redaction 확인
- 판정: `PASS`, `FAIL`, `BLOCKED`, `INCONCLUSIVE`

`PASS`는 한 장비에서 우연히 동작했다는 뜻이 아니다. 지정된 장비 조합과 실패 경로에서 같은 결과를 재현할 수 있어야 한다.

## 실험 목록

### E0. 환경 진단

목적: 장비별 입력과 세션 조건을 추측하지 않고 기록한다.

검증 항목:

- Windows/build와 CPU architecture
- 현재 사용자와 프로세스 권한 수준
- 활성 display, 해상도, 배율과 가상 좌표계
- 입력 장치 종류와 driver 식별 정보
- 잠금, 절전과 세션 변화 감지 가능 여부
- 입력 hook, Raw Input과 `SendInput` 사용 가능 여부

합격 조건:

- 비밀값이나 원시 입력을 노출하지 않고 후속 실험에 필요한 환경 차이를 식별한다.
- 지원하지 못하는 capability를 성공으로 위장하지 않고 명시적으로 보고한다.

### E1. 물리 입력 관찰

목적: 키보드와 마우스의 down/up 및 이동 event를 빠뜨리거나 중복하지 않고 정규화할 수 있는지 확인한다.

검증 항목:

- 내장 키보드, 터치패드와 외장 마우스
- key repeat와 modifier 조합
- 빠른 포인터 이동, 휠과 동시 버튼
- focus가 바뀐 상태와 전체 화면 응용프로그램
- 서로 다른 DPI와 배율

합격 조건:

- 각 물리 동작이 정의된 정규화 event로 일관되게 대응된다.
- down/up 짝이 맞지 않거나 중복되는 경우를 감지하고 증거로 남긴다.
- hook callback이 파일 I/O, 네트워크, UI 갱신 또는 blocking 작업을 수행하지 않는다.

### E2. 합성 입력 주입과 루프 차단

목적: `SendInput`으로 테스트 입력을 주입하고 그 입력이 다시 outbound 후보가 되지 않도록 구분한다.

검증 항목:

- 일반 key와 modifier의 down/up
- 마우스 이동, 휠과 버튼
- 주입 도중 실사용자의 물리 입력
- 합성 표식 누락, adapter 재시작과 callback 재진입

합격 조건:

- 합성 입력이 대상 응용프로그램에는 전달되지만 outbound 물리 입력으로 다시 분류되지 않는다.
- 어느 단계에서든 합성 여부가 불확실하면 전달 루프를 만드는 대신 입력 공유를 중단한다.
- 실험 종료 뒤 주입한 모든 key와 button이 해제된다.

### E3. 제한된 입력 억제와 자동 해제

목적: destination 준비가 확인된 상황을 흉내 내어 source 입력을 제한적으로 억제하고 안전하게 복구한다.

사전 조건:

- 독립 watchdog과 외부 복구 수단이 실제로 작동함을 먼저 확인한다.
- 억제 대상, 최대 활성 기간과 자동 해제 이유를 실행 전에 표시한다.

검증 항목:

- 정상 종료
- 실험 프로세스 강제 종료
- callback panic 또는 adapter 오류
- watchdog heartbeat 중단
- 긴급 해제
- 억제 중 들어온 물리 로컬 입력

합격 조건:

- destination 준비 전에는 어떤 입력도 억제하지 않는다.
- 정상과 비정상 종료 모두에서 로컬 입력이 자동으로 복구된다.
- 자동 복구가 실패하면 즉시 `FAIL`이며 다른 실험으로 진행하지 않는다.

### E4. 화면 경계 의도

목적: 포인터가 화면 끝에 닿은 것과 실제 전환 의도를 구분할 수 있는 최소 신호를 찾는다.

검증 항목:

- 좌우 경계 접근과 이탈
- 화면 모서리, taskbar와 scrollbar 사용
- 터치패드의 짧은 이동과 외장 마우스의 빠른 이동
- 서로 다른 배율과 해상도
- 경계에서 창을 조작하는 일반 동작

합격 조건:

- 경계 감도, dwell 또는 pressure 값은 측정 결과로 제안하며 미리 제품 상수로 고정하지 않는다.
- 오탐은 로컬 입력 억제로 바로 이어지지 않고 `PENDING` 단계에서 취소 가능하다.
- key 또는 button을 누른 상태의 전환은 별도 결과로 기록하고 MVP 허용 여부를 결정하지 않는다.

### E5. `PressedState`와 `ReleaseAll`

목적: 주입된 key와 button 상태를 복구 가능한 방식으로 추적하고 실패 뒤 모두 해제한다.

검증 항목:

- modifier를 포함한 key down 뒤 프로세스 종료
- mouse button down 뒤 연결 단절 모사
- down 기록 직전과 주입 직후의 crash window
- 중복 release와 이미 해제된 상태
- 물리 입력과 합성 입력이 동시에 들어오는 경우

합격 조건:

- crash window마다 남을 수 있는 상태와 복구 책임자가 문서화된다.
- `ReleaseAll`은 여러 번 실행해도 추가 입력을 만들거나 실제 물리 key 상태를 훼손하지 않는다.
- 복구 여부가 불확실하면 정상 완료로 보고하지 않는다.

### E6. lifecycle 장애

목적: Windows lifecycle 변화가 낡은 입력 상태나 억제를 남기지 않는지 확인한다.

검증 항목:

- 사용자 잠금과 잠금 해제
- 절전과 복귀
- 로그오프 또는 사용자 세션 종료
- display 분리 또는 해상도와 배율 변경
- adapter 시작 중 종료와 종료 중 재시작

합격 조건:

- lifecycle 변화가 감지되면 활성 실험 상태를 폐기하고 로컬 입력으로 돌아간다.
- 복귀 뒤 이전 lease, sequence 또는 눌림 상태를 재사용하지 않는다.
- 감지할 수 없는 lifecycle 구간은 지원 제한으로 기록한다.

### E7. 권한 경계

목적: 일반 사용자 프로세스가 실제로 제어할 수 있는 응용프로그램 범위를 확인한다.

검증 항목:

- 일반 사용자 응용프로그램
- 관리자 권한 응용프로그램
- UAC prompt
- Windows 보안 데스크톱

합격 조건:

- 동작하지 않는 권한 경계를 우회했다고 주장하지 않는다.
- 지원하지 않는 대상에서는 로컬 입력을 억제하지 않는다.
- 관리자 권한 agent나 service가 필요하면 편의 기능이 아니라 별도 보안 결정으로 올린다.

### E8. 장비 행렬과 장시간 안정성

목적: 한 개발 장비에서만 통하는 구현을 걸러낸다.

검증 항목:

- H-A, H-B, S-A, S-B 각각의 입력 장치
- 가능한 모든 source/destination 방향
- 터치패드와 외장 마우스
- 반복적인 시작, 중단, 잠금과 절전
- 높은 입력 빈도와 장시간 idle 뒤 재개

합격 조건:

- 모든 대상 장비의 결과가 기록된다.
- 반복 횟수와 실행 시간은 최초 측정으로 현실적인 값을 정한 뒤 성능 예산 문서에 출처와 함께 기록한다.
- 입력 손실, stuck key/button, 합성 루프 또는 복구 실패는 평균으로 희석하지 않고 개별 실패로 남긴다.

## 즉시 중단 조건

다음 중 하나라도 발생하면 억제와 주입 실험을 중단하고 원인을 해결하기 전까지 진행하지 않는다.

- 외부 복구 수단 없이 로컬 키보드나 마우스를 사용할 수 없게 됨
- watchdog이 억제를 해제하지 못함
- 재부팅 외에는 풀 수 없는 stuck key 또는 button
- 합성 입력이 재전달되며 반복됨
- 실제 입력 내용이 log나 support bundle에 기록됨
- 지원 범위를 만족하려면 kernel driver가 필요함
- 보안 데스크톱 우회나 권한 상승을 정상 기능으로 요구하게 됨

## 판정 규칙

### Go adapter 채택

순수 Go와 얇은 Win32 호출로 모든 안전 불변조건을 재현 가능하게 만족하고, callback 지연과 장비 차이가 허용 범위 안에 있으면 채택한다.

### native 모듈 연구

Go runtime 또는 callback 경계가 원인으로 특정된 누락, 지연이나 복구 실패가 있고 작은 C 또는 Rust 모듈로 격리할 수 있을 때만 연구한다. 전체 제품을 다른 언어로 옮기는 근거로 사용하지 않는다.

### 프로젝트 중단 또는 범위 축소

입력 억제를 fail-open으로 보장할 수 없거나 복구 책임자를 만들 수 없으면 자동 화면 경계 전환을 중단한다. 이 경우 명시적 단축키 전환, 입력 억제 없는 원격 제어 또는 기존 제품 사용을 대안으로 재평가한다.

## Phase 0 완료 산출물

- 채워진 장비 원장
- 실험별 결과와 redacted 증거
- 재현 가능한 실패 목록
- 입력 adapter 선택 ADR
- 지원 Windows/권한 범위 초안
- 성능 예산의 측정 기준
- Phase 1로 진행, 범위 축소 또는 중단 판정

## 현재 미검증 가정

- 네 대 모두 동일한 Windows generation과 CPU architecture를 사용한다.
- 스터디카페 네트워크가 두 기기 사이 직접 연결을 허용한다.
- 관리자 권한 설치 또는 방화벽 설정이 가능하다.
- key나 button을 누른 상태의 경계 전환을 MVP에서 금지할 수 있다.
- `SendInput`과 사용자 세션 agent만으로 기대하는 응용프로그램 범위를 충족한다.

이 가정들은 Phase 0 결과나 사용자 결정 없이 `Accepted` ADR로 승격하지 않는다.
