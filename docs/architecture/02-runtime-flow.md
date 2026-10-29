# 런타임 흐름

- 상태: Draft

## 시작

1. 단일 agent 인스턴스가 로컬 신원, 신뢰 peer와 설정을 읽는다.
2. Windows 입력 adapter와 로컬 projection API를 시작한다.
3. 연결이 없거나 peer 인증이 끝나지 않으면 `LOCAL`을 유지한다.

g478의 상대 관리 창 열기는 에이전트와 실행 기록의 generation/revision을 함께
확인한다. 불일치하면 로컬 peer-update staging에서 정확히 같은 빌드만 전체 검증하고
실행한다. 수신 권한은 실행 직전에 다시 확인하며, 메타데이터·EXE의 쓰기/교체와
폴더 이름 변경을 실행 완료까지 차단한다. 새 UI는 기존 인계·시작 성공 등록을 거치며,
이전 버전 창의 준비 보고로 현재 창의 `window_ready`를 대신하지 않는다.

## 경계 전환

1. Windows adapter가 경계 의도를 감지한다.
2. Application이 destination 연결, 세션과 제어권 가능 여부를 확인한다.
3. destination이 수신 준비를 확인한다.
4. source가 `OUTBOUND_ACTIVE`, destination이 `INBOUND_ACTIVE`가 된 뒤 입력 전달을 시작한다.
5. 반대 경계 전환은 같은 절차로 새 제어권을 만든다.

destination의 cursor entry는 return detector arm 뒤 `PrepareEntry`로 이동 전 좌표와 실제 entry 좌표를
기억한다. inbound runtime grant와 grant frame write가 모두 성공해야 `CommitEntry`한다. 그 전에 실패하면
취소를 상속하지 않는 bounded cleanup에서 `RollbackEntry`를 호출하며, 현재 cursor가 확인된 entry 좌표에
그대로 있을 때만 이전 좌표로 복원한다. 이미 움직인 cursor는 오래된 snapshot으로 덮어쓰지 않는다.

제어권 반환 시 destination의 커서 숨김은 네이티브 소유 스레드의 같은 메시지 큐에 snapshot과 show를
순서대로 게시한다. show는 revoke 전송 성공 후에만 게시한다. peer 스레드는 snapshot을 기다리지 않으며,
20ms 지연만으로 숨김 요청을 버리지 않는다. 최신 요청 ID, 입력 generation, 현재 데스크톱과 좌표를
확인하므로 이전 요청이나 반환 중 실제 입력은 뒤늦게 커서를 숨길 수 없다. 이미 숨긴 경계 좌표와 같은
이동 보고는 숨김을 유지하며, 좌표가 달라진 이동·클릭·휠·키 입력 또는 새 진입은 커서를 복원한다.
태그된 동일 좌표 보고는 네이티브 처리를 하지 않으므로 데스크톱 조회도 추가하지 않는다.
같은 좌표의 이동 보고가 snapshot과 show 사이에 도착한 경우에도, 현재 generation의 유효한
snapshot이 이미 있으면 대기 중인 숨김을 유지한다. snapshot 이전의 입력, 좌표 변경, 클릭·키 입력과
교체된 snapshot은 계속 대기 중인 숨김을 취소한다.

보호 캡처의 실행·경계 인증은 서비스와 에이전트의 독립적인 읽기 전용 증명을 같은 1초 예산에서
병렬 조회한다. 두 호출이 모두 반환하고 모두 성공해야 실행한다. 매번 새 증명을 얻으며, 실패한
증명이나 만료 뒤 남은 네이티브 조회의 자원 소유권·동시 실행 상한·생존 확인용 예약 슬롯은 유지한다.

보호 캡처의 명령 데스크톱이 실행 직전 새 권한 확인을 마쳤다면, 그 명령의 OS 스레드와
콜백 안에서 캡처 한 번만 추가 사전 조회를 생략한다. 표식은 콜백 반환 시 제거하고,
취소·다른 스레드·두 번째 사용·보관한 activation의 후속 호출에는 적용하지 않는다.
캡처 소유 스레드의 데스크톱 유효성 검사와 활성화 후 새 권한 확인은 그대로 수행한다.
사용자 지정 경계·화면 배치 검사는 활성화 전후 모두 유지하며 권한 증명을 캐시하지 않는다.

g482의 보호 캡처는 억제가 시작된 뒤 발생한 입력을 기존 bounded/coalescing 큐에 보관한다.
네이티브 활성화와 사후 권한 검사가 성공하고 실제 완료 ACK를 파이프에 쓴 뒤에만 송신자를 푼다.
대기 중인 입력은 활성화의 인증 조회나 응답 파이프를 차지하지 않는다. 각 입력의 새 권한 확인과
송신 직전 generation 재검사는 유지하며, 실패·취소·교체된 activation은 입력을 방출하지 않는다.
이 대기는 입력 콜백이나 heartbeat 응답을 막지 않고 기존 큐의 크기·이동 합산·키/버튼 순서를 유지한다.

사전 준비 힌트는 semantic coordinator를 통과해도 TLS의 별도 전송 게이트에서 대기할 수 있다.
g483은 transport가 `admission` 단계의 취소·만료로 명시한 미전송 힌트만 폐기하고 세션을 유지한다.
같은 힌트를 재전송하거나 입력 권한을 부여하지 않는다. 전송 시작 여부가 불명확한 오류,
deadline 설정 실패와 실제 TLS 쓰기 실패는 계속 연결을 종료하며 부분 전송을 숨기지 않는다.

g484의 입력 워커 heartbeat는 전송 잠금 밖의 별도 maintenance 조회에서 새 프로세스 생존 증명을
확인한다. 아직 전송하지 않은 조회는 1초에 중단하며, 네이티브 조회 핸들은 기존 bounded owner가
완료까지 보유한다. 조회 만료만으로 입력 채널을 폐기하지 않으며 실제 입력의 새 권한·presence
확인과 경쟁하지 않는다. 전송 직전에 잠금 안에서 채널 폐기·리스 만료와 최근 ACK를 다시 확인한다.
조회와 전송을 합친 기존 2초 제한은 늘리지 않는다. 전송한 요청의 ACK 유실·불일치, 확인된 소유자
실패와 입력 권한 실패는 계속 세션 종료 대상이며, heartbeat로 입력 권한이나 리스 시간을 연장하지 않는다.

g489는 일치하는 성공 ACK의 요청 전송 시작 시각을 최근 생존 확인의 기준으로 보관한다.
워커는 ACK를 쓴 직후 idle 기한을 갱신할 수 있으므로, 에이전트가 늦게 ACK를 처리한 시각은
워커 기한의 기준으로 사용할 수 없다. 요청 시작부터 250ms 이상 지났으면 유지용 heartbeat를
생략하지 않는다. 빠른 ACK의 중복 전송 생략, 잠금 전후 재검사, 기존 2초 idle 기한과 리스 만료는
유지하며 실패·불일치·유실 ACK는 이 시각을 갱신하지 않는다.

입력 워커가 READY 전에 종료하면 서비스는 READY와 최종 보고를 구별한다. 입력 처리 횟수·복구
저널이 비어 있고, 네이티브 소유자와 정확한 Job/process의 정리가 모두 확인된 보고만 에이전트에
전달한다. 에이전트는 이를 입력 허가로 쓰지 않고 해당 리스를 거절한다. 보고 유실·절단·다른 워커
종류·복구 불확실성은 계속 실패 처리한다. 준비된 미바인딩 자식도 최종 보고 전 정리를 완료해야 한다.

입력 네이티브 워커의 시작과 수동 커서 프레젠테이션도 `authorizeWorkerOwners`에서 서비스·에이전트의
새 증명을 같은 1초 이내에서 병렬 조회한다. 호출자·리스의 더 짧은 제한 시간이 우선하며, 두 호출을
모두 합류하고 성공한 뒤에만 워커를 시작한다. 한쪽 실패·취소·만료는 계속 실행을 거절하고,
진행 중인 네이티브 조회의 핸들은 해당 조회가 끝날 때까지 기존 소유자가 유지한다.

입력 서비스는 요청 수락 전후에 완료된 준비 작업만 비차단 회수한다. 요청 대기 중 워커가
완성·만료됐더라도 다음 인증된 준비 힌트를 처리하기 전에 회수하므로, 완료된 builder가
교체 준비를 막지 않는다. 미완료 작업은 실제 리스 입장 때 합류하며, 정리 실패는 인증된
채널도 닫고 복구 실패로 전파한다. g490은 준비 후보 수명을 3초로 둬 2.1초 힌트 갱신 간격보다 길게 유지한다. 새 힌트를 받아도 후보 수명이 750ms 이상 남으면 교체하지 않는다. 원래 만료 타이머를 연장하거나 유휴 중 주기적으로 준비하지 않는다.

입력 또는 수동 커서 리스의 실제 READY를 에이전트에 전달한 뒤, 다음 입력용 unbound 세트를
최대 하나만 비동기로 준비한다. 활성 입력 시간과 콘솔·네이티브 input/release 준비를 겹치되,
예비 세트에는 현재 리스·epoch·에이전트 핸들·입력 journal을 전달하지 않는다. 활성 세트와
예비 세트는 각각 별도 3-process Job에 속하며, 합계 최대 6개 프로세스가 일시적으로 존재한다.
g491은 startup 진단의 prepared-age 상한도 3초 수명과 맞춘다. 2~3초 된 정상 후보의 상세 계측이
이전 2초 검사 때문에 빠지지 않으며, 만료된 진단은 계속 버리고 리스 승인이나 입력 권한에 쓰지 않는다.
예비 후보가 3초 뒤 만료돼도 활성 리스 중 주기적으로 재생성하지 않는다. 정상 입력 종료와
정확한 기존 owner 정리가 끝나면 완료된 준비 결과를 회수하고, 살아 있는 후보는 재사용하며
만료된 후보는 종료 확인 후 한 번 교체한다. 미완료 builder는 그대로 다음 요청 인증과 겹치고,
예비 세트의 정리 실패는 다음 입장을 차단한다. 첫 진입·긴 유휴 후 생성 비용은 여전히 남는다.

상태 변경은 `internal/control` transition table에서만 결정한다. table이 반환한 effect는 Application이 순서대로 실행하고 성공 또는 실패 event를 다시 상태 머신에 넣는다. 현재 구현은 순수 transition core, authenticated peer transport, Raw Input과 low-level hook을 결합한 Win32 source, `SendInput` 기반 Win32 destination adapter를 포함한다.

활성 inbound lease에서 받은 입력은 실행 전에 `InputStream` gate를 통과한다. session, epoch 또는 정확한 다음 sequence가 하나라도 맞지 않으면 해당 payload를 실행하지 않고 `FAULT_DETECTED` cleanup으로 넘긴다. stream과 lease의 session이 서로 다르거나 non-local 상태에서 lease 방향과 맞지 않는 입력을 받은 경우도 동일하게 fail-safe 처리한다. 재연결은 새 session과 epoch, sequence `1`에서 시작한다.

Application coordinator는 transition이 반환한 effect를 순서대로 실행한다. 한 effect가 실패해도 같은 cleanup 묶음의 나머지 effect는 계속 시도한다. activation 또는 cleanup effect 실패는 `FAULT_DETECTED`로 환류하고, `ReleaseAll`이 성공하면 `RELEASE_ALL_SUCCEEDED`를 적용해 `LOCAL`까지 수렴한다. `RESUME_LOCAL`이 실패하면 같은 호출에서 무한 반복하지 않고 `RELEASE_ALL_FAILED`를 적용해 `FAULT`와 복구 상태를 유지한다.

heartbeat deadline은 caller가 제공한다. 만료된 현재-session liveness는 위 coordinator에 `FAULT_DETECTED`를 전달하며, 아직 살아 있는 session에는 state나 effect를 만들지 않는다. session 불일치, 시간이 뒤로 이동함, 잘못된 timeout처럼 liveness 판정을 신뢰할 수 없는 경우도 active lease를 유지하지 않고 같은 cleanup 경로로 보낸다.
수신 heartbeat는 coordinator를 획득한 뒤 읽은 local observation으로 기존 deadline을 먼저 검사한다. 이미 timeout에 도달한 session은 늦은 frame으로 갱신하거나 수신 evidence에 넣지 않고 held input을 해제한 뒤 `LOCAL`로 수렴한다.

ADR 0071에 따라 현재 inbound 입력도 action·session·epoch·sequence 검증과 주입 성공 뒤 liveness를 갱신한다. runtime owner 획득 후 기존 deadline을 먼저 검사하고, 주입 시작 전의 로컬 관찰 시각을 보존해 느린 주입 완료로 새 예산을 얻지 않는다. atomic lease watchdog도 같은 주입 시작 전 monotonic 시각을 사용한다. invalid·expired·실패 입력과 retired drain은 생존 근거가 아니다.

destination의 local return tombstone과 exact하게 일치하는 retired inbound session의 늦은 heartbeat는
`LOCAL`에서 오류로 승격하지 않고 drain한다. 이 drain은 liveness를 다시 활성화하거나 observation을
갱신하지 않으며 runtime은 `LOCAL`로 남는다. 다른 session의 heartbeat와 tombstone identity 불일치는
계속 오류로 처리해 stale session이 local runtime을 되살리지 못하게 한다.

## 장애와 종료

1. 추가 입력 억제와 전달을 중단한다.
2. 활성 lease를 폐기한다.
3. destination의 `PressedState`에 대해 `ReleaseAll`을 수행한다.
4. 입력 adapter와 연결을 닫는다.
5. 양쪽 상태를 `LOCAL`로 만든다.

순서를 확정할 수 없는 연결 복구에서는 미전달 입력을 재생하지 않는다. GUI 종료는 agent 종료와 같은 뜻이 아니며, agent 종료 요청은 위 안전 종료 절차를 반드시 거친다.

Phase 0 `agent-session`의 process owner는 숨겨진 top-level window의 Windows lifecycle monitor,
connection source와 crash guard를 함께 소유한다. suspend, resume, session lock·disconnect, logoff,
shutdown과 display change의 첫 event는 terminal cancellation이며 기존 session을 resume하지 않는다.
supervisor cleanup이 끝난 뒤 source, crash guard, monitor 순으로 닫고 다음 실행에서 diagnostics와
명시적 rearm을 다시 수행한다. monitor 자체의 예기치 않은 종료도 fail-closed failure다.

현재 Application 구현은 in-memory snapshot과 injected adapter port를 대상으로 한다. `SessionRuntime`은 lease transition, input sequence 승인, pressed-state 변경, 실제 key/button 주입과 주입 실패 cleanup을 하나의 직렬 임계구역에서 수행한다. heartbeat 관찰과 만료 판정도 같은 owner를 거친다.

정규화된 pointer move와 wheel action도 같은 sequence와 owner 경계를 통과한다. action 형식 오류나 adapter 주입 실패는 해당 payload를 재생하지 않고 session fault cleanup으로 수렴한다.

Win32 destination adapter는 keyboard scan-code down/up, left/right/middle/X1/X2 button down/up, relative pointer move와 vertical/horizontal wheel을 `SendInput`으로 주입한다. 두 wheel 축이 한 action에 있으면 vertical 다음 horizontal 순서의 한 batch로 보낸다. adapter 오류는 `NOT_APPLIED`, `PARTIALLY_APPLIED`, `OUTCOME_UNKNOWN` 확실성을 가진다. 호출 전 취소와 zero insertion은 새 press journal만 rollback하고, 부분·불명 결과와 기존 token의 반복 press는 cleanup owner를 유지한다. release는 실제 up 성공 뒤에만 journal에서 제거한다. Phase 0 report는 실패 확실성과 cursor rollback 결과를 aggregate count로만 남기며, 실패가 있거나 rollback 분류 합계가 맞지 않으면 통과하지 않는다. 합성 입력에는 Krasis tag를 넣지만 보안 판단은 tag 하나가 아니라 Windows injected flag와 현재 session 방향을 함께 사용해야 한다.

lab self-test의 tagged `+1/-1` 상대 이동은 두 injection이 hook으로 돌아오는지 검증하는 probe이며, Windows pointer acceleration과 pixel rounding 때문에 두 delta가 좌표상 정확히 상쇄된다고 가정하지 않는다. 두 event 뒤 좌표가 남으면 `SetCursorPos`로 시작 좌표를 복구하고 그 결과까지 확인한다. 이 복구는 명시적인 bounded self-test에만 사용하며 제품의 일반 pointer 전달 경로에는 사용하지 않는다.

lab `touchpad-surface`는 production input owner 밖의 별도 진단이다. Windows 11 visible foreground
window를 touchpad-capable로 등록하고 `WM_POINTER`의 `PT_TOUCHPAD` 분류와 legacy wheel을 aggregate로
관찰하지만 suppression, source queue, transport와 destination injection에 연결하지 않는다. pointer
message를 `DefWindowProc`에 전달해 기본 변환 경로를 깨지 않으며, 그 과정에서 관찰된 legacy wheel의
device나 변환 원인은 단정하지 않는다. 이 진단 결과와 실제 gram13 관찰을 근거로 ADR 0023이 active
source의 wheel owner를 foreground touchpad-capable window로 변경하며, 진단 명령 자체는 production
source queue나 transport에 연결하지 않는다.

source keyboard mapper는 physical keyboard hook의 scan code와 extended flag를 canonical key code로 바꾼다. mouse의 relative move와 button은 `RAWMOUSE` packet 하나에서 move, button 순서로 normalized action을 만들고 Precision Touchpad의 zero device handle도 정상 packet으로 허용한다. vertical/horizontal wheel은 Raw Input에서 만들지 않는다. active outbound에서는 touchpad-capable foreground capture window가 `DefWindowProc` 변환 뒤 받은 `WM_MOUSEWHEEL`과 `WM_MOUSEHWHEEL`만 signed delta payload로 정규화한다. low-level hook의 physical·untagged synthesized wheel은 aggregate 관찰 뒤 window로 통과하고 Krasis/self-test tag wheel만 삼킨다. capture foreground activation은 정상 `SetForegroundWindow`와 즉시 검증을 먼저 사용한다. 이 경로가 거절되거나 검증을 잃은 경우에만 capture thread와 현재 foreground thread를 잠시 attach하고 한 번 재시도한 뒤 항상 detach한다. 어느 단계든 실패하면 capture를 활성화하지 않고 ADR 0027의 abort/rollback으로 수렴한다. return deactivation도 정상 saved-focus 복원을 먼저 사용하고 거절·검증 상실에만 capture thread와 saved-focus window thread를 잠시 attach해 한 번 재시도한 뒤 항상 detach한다. 정확한 복원이 실패하면 capture window를 숨긴 뒤 foreground release를 확인한다. capture가 foreground를 놓은 recovered·unavailable 결과는 degraded evidence로 남기고 session을 유지하며, hidden capture retention과 thread detach 실패만 terminal이다. capture가 active 상태에서 foreground ownership을 잃은 짧은 구간에는 low-level hook mapper로 fail-safe하고 곧 source-hook failure로 session을 종료한다. Raw Input wheel flag는 active-session 진단 aggregate만 갱신하며 suppression, mapping 또는 send ownership을 갖지 않는다. mouse hook의 화면 좌표는 경계 위치 관찰과 작은 capture window 배치에만 사용하고 좌표 차이를 원격 delta로 만들지 않는다. Krasis 또는 self-test tag가 있는 hook event와 known tagged Raw Input은 다시 전송하지 않는다.

source session은 input owner thread와 단일 network sender 사이에 고정 용량 queue를 둔다. keyboard와 Raw Input action은 현재 outbound generation에 non-blocking enqueue하며 sender는 action 순서를 보존한다. discrete keyboard suppression은 해당 action offer가 성공한 경우에만 적용한다. mouse는 Raw Input 수신과 hook callback을 원자적으로 묶을 수 없으므로 active generation에서 physical hook event를 먼저 억제하고 Raw Input action을 별도로 offer한다. Raw read, parse 또는 queue가 실패하면 전달을 추측하거나 재생하지 않고 suppression과 generation을 폐기한다. sender 실패 또는 취소는 connection close interrupt, sender join과 runtime fault cleanup 전체를 하나의 bounded cleanup deadline 안에서 수행한다. `Close`나 cleanup 구현이 context를 무시해도 parent는 deadline 뒤 terminal error로 빠져나오며 늦은 completion은 buffered channel 밖으로 투영되지 않는다. 폐기된 generation의 queued action은 원격으로 보내지 않는다. queue 용량은 설정 가능한 `1..65536` 범위이며 현재 Phase 0 기본값은 4096이다.

normalized discrete input의 code domain은 `PressedToken.Validate`가 단일 source of truth로 소유한다. `InputAction.Validate`는 key와 button action을 token으로 변환해 같은 검증을 수행하므로 source queue, wire encoder, wire decoder와 destination runtime이 zero scan code, 지원하지 않는 keyboard bit와 `1..5` 밖의 mouse button을 동일하게 거부한다.

`SourceHook`은 세 고정 OS thread를 소유한다. 첫 thread는 `WH_KEYBOARD_LL`과 `WH_MOUSE_LL` 전용 message loop, 둘째는 message-only Raw Input window와 `WM_INPUT` read, 셋째는 숨은 touchpad-capable wheel capture window를 소유한다. 세 surface가 모두 ready인 뒤에만 boundary orchestrator를 시작한다. Raw Input queue에 쌓인 packet은 8-byte aligned 64KiB 재사용 buffer로 batch drain하지만 hook dispatch를 막지 않는다. mouse hook callback이 갱신하고 Raw Input·capture thread가 읽는 최신 screen X는 atomic state이며, hook/Raw의 stateful boundary observation은 짧은 mutex로 직렬화한다. active capture activation과 deactivation은 message command와 bounded ACK로 직렬화한다. active focus loss, 등록 해제 실패, hidden capture retention과 thread detach 실패는 source-hook component failure지만, 정확한 saved-focus 복원만 실패하고 capture foreground release가 확인되면 hook을 유지한다. foreground fallback의 attach, retry와 detach도 이 고정 capture thread에서 직렬 실행하고 retry loop를 만들지 않는다. detach 실패는 thread 종료로 수렴하는 terminal failure이며 HWND와 thread ID는 report에 남기지 않는다. keyboard callback, window wheel mapper, Raw Input mapper, `BoundaryIntentEmitter.Observe`, `SourceSession.Offer`는 network I/O를 수행하지 않는다. Win32 hook data 역참조와 hook mapper는 원격 enqueue 전에 별도 panic 경계를 유지한다. mapper 또는 boundary observer가 panic하면 payload를 노출하지 않는 고정 오류로 변환하고 source generation을 폐기한다. boundary intent handoff는 용량 1 queue에 non-blocking으로 수행하며 이미 intent가 대기 중이면 새 intent를 버린다. Raw Input registration은 process당 mouse device class 하나만 허용되므로 제품 source, pointer shadow와 lab capture는 process-global hook gate를 공유한다.

`OutboundController`는 boundary orchestrator와 control dispatcher가 공유하는 source-side lifecycle owner다. boundary intent에서는 `LeaseRequest`만 보내고 source suppression을 켜지 않는다. destination은 cursor와 return detector를 prepare하고 `INBOUND_PENDING`에서 `LeaseGrant`를 쓴다. source는 exact session·lease·epoch의 grant를 적용한 뒤 `SourceSession`과 touchpad capture를 활성화한다. 준비 성공 시 ordered connection에 `LeaseCommit`을 먼저 쓰고 destination은 그 commit 뒤에만 `INBOUND_ACTIVE`와 cursor entry를 확정한다. 준비 실패는 `LeaseAbort`로 양쪽 prepared state를 rollback한다. commit/abort outcome unknown, reject, revoke와 session failure에서는 suppression을 먼저 끄고 pending cursor를 rollback한 뒤 runtime cleanup으로 수렴한다.

`BoundaryOrchestrator`는 용량 1 intent queue를 순차 소비하고 각 새 outbound 요청에 `crypto/rand` 기반 128-bit session ID와 별도 128-bit epoch를 생성한다. runtime이 `LOCAL`이 아니거나 source suppression이 이미 활성화된 동안 도착한 intent는 현재 session을 건드리지 않고 버린다. entropy 생성 실패와 최초 request write 실패는 orchestrator error로 supervisor에 전달한다.

`LeaseRequest`는 source가 넘은 `LEFT` 또는 `RIGHT` edge를 포함한다. destination `CursorEntryPort`는 승인된 `RIGHT` request를 virtual desktop 왼쪽에서 1px 안쪽, `LEFT` request를 오른쪽에서 1px 안쪽, 세로 중앙에 배치한다. cursor 배치가 성공한 뒤에만 inbound runtime을 활성화하고 grant를 쓴다. authorization reject는 cursor를 움직이지 않는다. 배치 snapshot과 readback을 확보한 좌표 불일치는 grant 전에 조건부 rollback이 정상 완료된 경우에만 해당 요청을 `LeaseReject`로 거절하고 연결을 유지한다. 결과 불명, rollback 실패·미분류, 취소 또는 reject 전송 실패는 계속 session failure로 처리한다. 입력을 재생하거나 커서 배치를 재시도하지 않는다. 세부 계약은 [진입 불일치 거절](../experiments/cursor-entry-mismatch-rejection.md)에 기록한다.

destination input adapter는 현재 inbound session·epoch로 return detector를 arm하되 entry 직후에는
return-ready로 보지 않는다. source `RIGHT` 진입은 destination 왼쪽에서 오른쪽으로, source `LEFT`
진입은 destination 오른쪽에서 왼쪽으로 configured rearm distance 이상 이동한 뒤 entry edge로 돌아와야
return intent를 non-blocking으로 한 번만 발행한다. 오래된 queued intent는 새 session arm에서 폐기하고 runtime은 현재 inbound session·epoch가 정확히 일치하며 pressed input이 없을 때만 local revoke를 승인한다. 승인된 revoke는 destination을 먼저 `LOCAL`로 cleanup한 뒤 exact session·lease·epoch의 `LeaseRevoke`를 쓰며 source는 이를 받은 뒤 suppression을 해제한다. intent와 button-down의 경합은 session failure가 아니라 detector rearm으로 처리한다.

성공한 local return은 폐기한 inbound stream의 exact session·epoch와 다음 sequence를 tombstone으로 남긴다.
revoke가 반대 방향으로 전달되기 전에 이미 source writer에 들어간 연속 input은 `LOCAL`에서 주입하지 않고
tombstone sequence만 전진시킨다. 새 inbound grant는 tombstone을 폐기하며 다른 session·epoch, duplicate와
sequence gap은 계속 protocol failure다.

source `RunSession`의 EOF, cancellation, malformed frame과 dispatcher failure는 outbound lifecycle의 `Fail`을 통과해야 한다. 이 경로는 source suppression을 먼저 해제한 뒤 bounded runtime cleanup을 수행한다. runtime만 `LOCAL`로 돌리고 hook suppression을 남기는 종료 경로는 허용하지 않는다.

runtime은 port 호출이 끝날 때까지 owner lock을 유지한다. 따라서 effect와 input port는 같은 runtime API로 재진입하지 않아야 하며, 느린 I/O를 직접 수행하는 대신 adapter 내부의 명확히 제한된 호출만 제공해야 한다. transport와 Windows adapter는 이 runtime 경계를 통해서만 event와 input을 전달한다.

durable state commit, optimistic version compare-and-swap, transition history와 command
idempotency는 아직 후속 경계다. Phase 0 `agent-session`의 destination discrete input은 별도
crash guard에 down 의도를 ACK 받은 뒤 주입하고, 성공한 up 뒤 guard 상태를 제거한다.
parent pipe EOF, protocol 오류와 정상 disarm에서 guard가 남은 token을 역순 해제한다.
IPC ACK와 process 종료는 각각 2초 안에 끝나야 하며 timeout은 session fault다.
guard는 종료 reason과 aggregate release count를 별도 no-replace JSON에 기록한 뒤 종료한다.
정상 agent report는 이 파일의 basename만 참조하며 checker가 같은 directory 안에서
device/build와 `DISARM` zero-recovery를 대조한다. parent crash evidence는 `PARENT_EOF`와
실제 release count를 요구한다.
subprocess test는 parent 강제 종료 뒤 이 독립 cleanup owner가 실행됨을 증명하지만 실제
실제 `SendInput`, 물리 입력 동시성, guard와 agent 동시 종료 또는 전원 차단은 증명하지 않는다.

## Peer transport

MVP transport framing은 `docs/adr/0003-ordered-peer-transport.md`를 따른다. 하나의 ordered stream에서 bounded frame을 순서대로 읽고, transport는 control, input, heartbeat kind와 payload 크기까지만 검증한다. payload decode와 runtime 호출은 인증된 connection을 소유한 adapter 책임이다.

server와 client TLS config는 TLS 1.3만 허용하고 `krasis/1` ALPN을 사용한다. server는 신뢰 root로 검증된 client certificate를 요구하고 client도 명시적인 expected server name과 신뢰 root로 server certificate를 검증한다. 인증이 끝나면 leaf certificate의 SHA-256 fingerprint와 EKU 기반 역할을 `AuthenticatedPeerContext`로 만들고, `PeerConnection`에서 agent session과 `LeaseAuthorizer`까지 필수 전달한다. fingerprint는 certificate subject나 원문 인증서를 로그에 복사하지 않는 stable identity다. system root fallback, insecure skip verify와 익명 연결은 사용하지 않는다.

`PeerConnection`은 mutual TLS와 ALPN 검증이 완료된 뒤에만 bounded frame reader/writer를 노출한다. writer 호출은 connection 안에서 직렬화해 여러 producer의 frame bytes가 섞이지 않게 한다. input dispatcher는 payload decode가 실패하면 활성 `SessionRuntime`을 fault cleanup하고, router가 잘못 전달한 non-input frame은 runtime state를 건드리지 않고 거부한다.

## 상주 agent session 조립

`internal/agent.RunSession`은 인증이 끝난 peer connection 하나에 다음 lifecycle을 한꺼번에 묶는다.

- `SourceSession` worker를 먼저 시작하고 ready 이후에만 low-level source hook과 boundary orchestrator를 시작한다.
- 같은 `SessionRuntime`과 connection을 outbound controller, inbound lease responder, return controller가 공유한다.
- 한 peer가 경계를 넘으면 source suppression은 exact lease grant 뒤에만 활성화된다.
- 반대 peer가 경계를 넘으면 같은 process가 destination input adapter와 return controller로 동작한다.
- source worker, hook, boundary orchestrator, return controller, peer reader 중 하나가 비정상 종료하면 공통 context를 취소하고 나머지를 모두 회수한다.
- worker가 반환한 `context.Canceled` 또는 `context.DeadlineExceeded`도 공통 session context가 이미
  취소된 경우에만 정상적인 teardown 결과로 취급한다. session context가 살아 있는데 component
  자체 취소나 deadline으로 종료하면 component failure로 승격해 나머지 worker를 회수한다.
- 종료 결과가 `LOCAL`이 아니면 정상 종료로 보고하지 않는다.

`internal/agent.RunSupervisor`는 이 session 조립 경계 바깥에서 listen 또는 dial connection source를 반복 실행한다. supervisor 상태는 `STOPPED`, `ACQUIRING`, `ACTIVE`, `BACKOFF`이며 transition table에 없는 전이는 거부한다. connection 획득 실패와 active session 실패는 서로 다른 failure kind로 관찰되지만 주소, certificate, session ID와 원시 오류 문자열은 snapshot에 넣지 않는다.
supervisor가 시작된 뒤 어떤 terminal return을 만나도 connection source를 닫고 마지막 observer
snapshot을 `STOPPED`로 수렴시킨다. connection source가 acquisition error와 함께 부분 connection을
반환하면 retry 전에 그 connection도 닫아 owner 없는 transport를 남기지 않는다.
session owner가 이미 관찰해 session error에 포함한 동일 connection Close 오류는 `LOCAL` cleanup 뒤
retry를 막지 않는다. session error와 독립적인 Close 오류와 partial acquire Close 실패는 terminal이다.
session component의 최초 failure는 stable component 이름으로만 observer에 투영하고 lab observer도 첫
non-empty component를 고정해 이후 retry failure로 덮어쓰지 않는다. 내부 error cause는
cleanup과 `errors.Is` 판단에 보존하지만 report에는 원문을 복제하지 않는다. source wheel은 physical hook,
Raw와 active capture window 관찰, mapping, queue accepted/rejected 사유와 successful send를 aggregate
pipeline으로 남긴다. v13은 `rawObserved`, `activeHookMoves`, `activeHookButtons`, `windowObserved`, capture
activation/deactivation과 focus restore 분류를 집계한다. source action이 있으면 capture activation이
필수이고 clean report는 activation/deactivation과 focus 결과 합계가 맞아야 한다. restore failure가 있으면
post-hide release outcome으로 모두 분류되고 hidden capture retention과 thread detach failure가 없어야 한다.
이 counter들은 같은 OS event를 correlate하지 않아 exact equality나 Raw payload ownership을 뜻하지 않는다.
`rawObserved>0` 또는 `windowObserved>0`인데 `sent==0`이면 해당 stable unforwarded code로 실패한다. 폐기된
generation의 이미 시작된 send가 늦게 실패하면 현재 generation을 종료하지 않고 버리며, current
generation send 실패만 source session failure다.
active connection은 supervisor가 씌운 close-once owner를 session과 공유한다. 두 경계가 동시에
종료를 시도해도 실제 transport Close는 한 번만 실행되고 첫 Close 결과가 보존된다. connection,
listener 또는 TLS 인증 실패 뒤 raw socket Close가 실패하면 기존 종료 원인과 close 오류를 함께
호출자에게 반환하며, 닫힘이 불확실한 connection으로 retry하지 않는다.

재시도 간격은 첫 250ms부터 두 배로 증가하고 30초에서 제한된다. session이 30초 이상 유지된 뒤 실패하면 다음 시도는 첫 간격으로 되돌아간다. 이 값은 현재 단일 peer 실험 기본값이며 실제 장비 측정 전 제품 SLO가 아니다. jitter는 한 listener와 한 connector만 있는 MVP topology에서는 아직 적용하지 않는다.

supervisor는 session 종료 결과가 `LOCAL`일 때만 새 connection을 얻는다. cleanup이 끝나지 않아 `FAULT` 또는 다른 상태에 남으면 reconnect로 덮지 않고 supervisor 자체를 실패시킨다. context 취소, 예상 밖의 clean session return과 unsafe final state를 포함한 모든 terminal 종료는 listener와 active connection을 닫고 `STOPPED`로 수렴한다.

source hook 앞단의 emergency observer는 physical `Ctrl+Alt+Shift+F12`를 감지한다. modifier event는 active session 중 이미 전달됐을 수 있으므로 chord terminator에서 suppression을 먼저 끄고 session fault cleanup을 시작해 destination의 `ReleaseAll`을 유도한다. F12 event 자체는 remote queue에 넣지 않고 로컬 hook chain으로 넘긴다. supervisor는 `ErrEmergencyRelease`를 terminal operator event로 분류해 reconnect하지 않는다. 이 chord는 Phase 0 lab 계약이며 제품 설정 정책은 아직 미결정이다.

`ListenSource`와 `DialSource`는 TCP와 TLS 세부사항을 supervisor core 밖에 둔다. listener는 accept loop와 pre-auth TLS handshake를 분리하고 최대 4개의 handshake만 동시에 수행하며, 각 handshake를 5초 안에 끝내지 못하면 socket을 닫는다. slot이 모두 찬 동안 새 TCP connection은 즉시 닫고, supervisor가 connection을 기다리지 않는 동안 들어온 connection도 닫아서 다음 session으로 넘기지 않는다. 둘 다 `PeerConnection` 생성 시 TLS 1.3 mTLS와 ALPN 인증을 통과해야 connection을 application에 넘긴다. credential storage와 IP별 rate limit은 transport 경계가 소유한다. 제품 `krasis-agent.exe`는 session-local 단일 instance와 authenticated loopback API를 소유하고 `krasis.exe`는 그 API만 투영한다. product network session adapter와 tray는 #15 및 후속 activation 작업에 남아 있다. 따라서 Phase 0 listener는 신뢰하는 사설 LAN이나 개인 hotspot에서만 열고, 공용 LAN에서는 Windows 방화벽으로 connector 주소만 허용하지 못하면 실행하지 않는다. launcher는 방화벽 규칙을 자동 생성하지 않는다.

control payload version 3은 wire 전용 `LeaseRequest`, `LeaseGrant`, `LeaseReject`, `LeaseCommit`, `LeaseAbort`, `LeaseRevoke` 명령만 허용한다. 모든 명령은 session과 epoch를 포함하고 grant, commit, abort와 revoke는 lease를 포함한다. `FAULT_DETECTED`, `RELEASE_ALL_SUCCEEDED` 같은 내부 상태 머신 event는 wire command가 아니며 peer가 직렬화하거나 주입할 수 없다.

인증된 peer session read loop는 control, input, heartbeat frame을 한 ordered stream에서 순차 dispatch한다. payload 오류, 잘못된 방향, handler 실패, EOF와 context 취소는 모두 connection을 닫고 runtime의 fault cleanup을 호출한다. EOF는 정상적인 제어 종료로 취급하지 않으며 명시적인 revoke 없이 연결이 끝난 경우에도 `ReleaseAll` 후 `LOCAL`로 수렴해야 한다.
peer session은 cancellation callback, dispatch failure와 함수 종료가 공유하는 close-once owner로
connection을 닫는다. 실제 Close는 한 번만 실행하고 첫 오류를 보존하며, EOF·context cancellation·
runtime cleanup 원인과 connection Close 오류를 함께 호출자에게 반환한다. agent supervisor가 다시
Close를 요청해도 같은 owner 결과를 사용하고 닫힘 실패가 포함된 session은 reconnect하지 않는다.

frame writer의 byte mutex만으로 runtime transition과 frame의 의미 순서는 보장되지 않는다.
ADR 0005의 `SessionWriteCoordinator`는 grant, reject, revoke, input과 heartbeat의 state
재확인·예약·write 순서를 함께 소유한다. read 또는 dispatch failure는 connection을 먼저 닫아
막힌 write를 깨운 뒤 같은 coordinator에서 bounded cleanup하며 source suppression은 lock 대기
전과 획득 뒤에 모두 해제한다. heartbeat decode, liveness domain과 coordinated send primitive는
구현됐다. caller가 명시적인 interval과 더 긴 timeout policy를 주입하면 optional worker가 같은
coordinator를 획득한 뒤 fresh local monotonic observation을 읽어 deadline 확인과 heartbeat
write를 수행한다. coordinator 대기 전 ticker 시각은 deadline 근거로 재사용하지 않는다.
deadline fault cleanup은 session cancellation을 떼어낸 bounded context를 사용해 held input
release가 동시 취소에 잘리지 않게 한다.
bounded cleanup이 coordinator를 획득하기 전에 timeout되면 runtime state를 coordinator 밖에서
추측해 바꾸지 않는다. 오류에는 원래 session fault와 cleanup deadline을 함께 남기며 non-`LOCAL`
snapshot 때문에 supervisor는 reconnect를 중단한다.
cleanup operation이 실행됐지만 `FAULT`에 남고 context가 살아 있으면 state machine의 기존 fault
recovery transition을 같은 coordinator 안에서 한 번 더 실행한다. 성공한 release token은
`PressedState`에서 이미 제거됐으므로 남은 token만 재시도한다. 두 번째 시도도 실패하면 `FAULT`와
남은 token을 보존하고 supervisor가 종료하며 무한 retry는 만들지 않는다.
성공한 heartbeat write와 runtime observation 뒤에는 optional observer가 aggregate evidence를
기록한다. observer는 coordinator 밖에서 호출되고 session ID를 report에 투영하지 않으며,
새 session에서는 receive gap baseline을 다시 시작한다.
policy가 없으면 worker는 없고
제품 기본값과 `agent-session` evidence 계약은 여전히 `UNDECIDED`다.
inbound input과 heartbeat dispatch는 coordinator 안에서 runtime을 변경하고 malformed control은
connection close 뒤 cleanup하므로 fault 전이 뒤 semantic frame이 성공적으로 기록되지 않는다.

wire 송신은 상태 머신 effect executor가 아니라 peer session orchestrator가 runtime 전이 성공 뒤 수행한다. `BeginOutboundSession`은 먼저 `PENDING`을 확정한 뒤 `LeaseRequest`를 쓰며, write 실패 시 pending session을 폐기한다. destination의 `LeaseResponder`는 transport가 검증한 `AuthenticatedPeerContext`와 request metadata를 authorization policy에 함께 넘기고, policy 결과에 따라 reject를 보내거나 inbound runtime을 활성화한 뒤 grant를 보낸다. Phase 0 `agent-session`은 `krasis.trust-store/v1` document를 시작 시 strict하게 검증하고 각 inbound lease authorization에서 protected store를 다시 읽어 fingerprint, 역할, `AllowInboundControl`과 revocation 상태를 검사한다. reload 오류는 기존 allowlist로 되돌아가지 않고 fail-closed한다. 이미 active인 lease를 file watcher로 비동기 종료하지는 않으며 폐기는 다음 inbound lease부터 적용한다. `peer.pem`은 `trust import`에서 identity source로만 사용한다. grant write가 불확실하면 destination도 즉시 cleanup한다. input write는 runtime이 sequence를 예약한 뒤 수행하며 write 결과가 불확실하면 sequence를 재사용하지 않고 session 전체를 폐기한다.

Ordinary input admission takes one fresh console observation while the login-owner gate is held. `Owner.Retire` reports whether an existing pre-login owner required cleanup under its own lock. Only actual retirement requires the second console observation; its failure, a changed console, a fenced/closed owner or cancellation still rejects admission. Every later lease/input authorization remains independent; no observation is cached between requests.

A crashguard child owns one bounded 128-byte ACK reader for its complete process lifetime. Both the guard ledger client and native input exchanges consume that same reader under Process.callMu; input exchanges never read raw stdout or create a per-exchange buffer. Process stdout remains the underlying close/abort owner. Buffering changes no ACK validation, command sequencing, cancellation budget or native-retirement proof.

Approved entry bounds its optional preparation-completion wait with an independent timer for min(remaining hint lifetime, 300ms), including the exactly-300ms boundary. The completion signal still means joined native cleanup, not deadline expiry. Timer/context expiry requests cancellation but retains the preparation owner and input mutex; ordinary entry must still acquire that mutex and confirm cleanup before opening a new worker. This bounds the optional completion wait, not the entire native-retirement or handoff duration.
