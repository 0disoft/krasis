# ADR 0018: low-level hook과 Raw Input은 서로 다른 OS thread가 소유한다

- 상태: Accepted
- 책임자: cherr
- 범위: Windows source input thread, message loop와 boundary observation 직렬화
- 후속 관계: ADR 0013과 ADR 0017의 hook-owned wheel lifecycle을 보완한다

## 맥락

2026-07-15 gram13→HP15 v11 실험에서 Raw Input pointer 628회와 button down/up 각 14회는 source에서
전송됐고 destination은 pointer 627회와 button down/up 각 14회를 주입했다. 반면 gram13의
`hookObserved`, `synthesizedObserved`, `mapped`, queue와 `sent`는 모두 0이었으며 두 손가락 scroll은
gram13 로컬 창에 적용됐다. HP15의 `destinationInput.wheels`도 0이었다.

분류 이전의 두 hook 관찰 수가 모두 0인데 target window가 scroll됐으므로 wheel flag 분류나 transport가
원인이 아니다. 기존 `SourceHook`은 `WH_KEYBOARD_LL`, `WH_MOUSE_LL`과 message-only Raw Input window를
하나의 OS thread에 설치하고, 각 `WM_INPUT` 뒤 `GetRawInputBuffer`를 queue가 빌 때까지 반복했다.
Windows low-level hook은 설치 thread에 message를 보내 callback을 호출하며 timeout되면 Windows 7
이상에서 통보 없이 제거될 수 있다. 지속 Raw Input drain과 hook dispatch를 같은 message loop에 두면
Raw Input은 계속 동작하면서 hook만 사라지는 부분 실패를 만들 수 있다.

## 결정

- `WH_KEYBOARD_LL`과 `WH_MOUSE_LL`은 low-level hook 전용 고정 OS thread와 message loop가 소유한다.
- Raw Input registration, message-only window, current packet read와 buffered drain은 별도 고정 OS
  thread가 소유한다.
- 두 thread가 모두 ready message를 처리한 뒤에만 `SourceHook` 전체 ready callback을 호출한다.
- 어느 thread든 비정상 종료하면 공통 child context를 취소하고 다른 thread를 회수한다. parent context
  취소와 component failure를 구분하고 failure에는 `low-level-hook` 또는 `raw-mouse` 경계를 보존한다.
- child 회수는 2초 cleanup deadline을 공유한다. deadline 안에 모두 끝나지 않으면 `thread-join`
  failure로 종료하되 process-local hook session 소유권은 해제하지 않아 같은 process의 retry가 두 번째
  hook 집합을 설치하지 못하게 한다.
- hook·Raw Input·wheel 창의 message loop는 취소 이벤트와 메시지를 함께 기다린다.
  `WM_QUIT` 게시나 queue가 비기를 기다리지 않고, 메시지 사이에도 취소를 확인한다.
  `MsgWaitForMultipleObjectsEx`의 `MWMO_INPUTAVAILABLE`을 사용해 이미 확인한 미처리 메시지도
  깨움 대상으로 유지한다. 이벤트 신호 실패에는 최대 250ms의 native wait fallback이 있다.
  이벤트는 신호 goroutine을 join한 뒤 닫고, 실제 native owner 회수 규칙은 그대로 유지한다.
- cleanup timer와 완료 통지가 함께 준비되면 deadline 이전 완료 시각이 기록된 통지를 먼저
  회수한다. 실제 지연·미회수는 실패로 남기고 `thread-join-low-level-hook`,
  `thread-join-raw-mouse`, `thread-join-wheel-capture`, `thread-join-multiple`로 구분한다.
- hook thread가 갱신하고 Raw Input thread가 읽는 최신 pointer X는 atomic state로 공유한다.
- 두 thread가 하나의 stateful boundary detector를 동시에 호출하지 않도록 boundary observer 전체를
  짧은 mutex로 직렬화한다. detector와 bounded queue offer는 계속 blocking I/O를 수행하지 않는다.
- Raw Input buffered drain 자체는 high-frequency packet 처리를 위해 유지한다. 이 drain은 hook thread의
  message dispatch를 더 이상 막을 수 없다.

## 결과

1000Hz Raw Input backlog가 있어도 low-level hook message loop는 독립적으로 dispatch된다. keyboard와
wheel suppression이 Raw Input move·button 처리량 때문에 Windows timeout에 걸리는 구조적 경로를
제거한다. pointer·keyboard·wheel action이 boundary detector에 들어가는 전역 순서는 mutex 획득 순서이며
물리 장치의 절대 시간 순서를 보장하지 않는다. ordered transport는 기존처럼 source queue에 들어온 뒤의
순서만 보장한다.

Windows wake message 게시 실패나 child cleanup 정지는 parent를 무기한 join시키지 않는다. 다만 OS
thread를 강제로 종료하지 않으므로 cleanup deadline failure 뒤 정상 재무장은 process 재기동이 필요하다.

Windows는 제거된 low-level hook을 직접 알려주지 않으므로 실제 gram13에서 `hookObserved` 또는
`synthesizedObserved`, `sent`, HP15 `destinationInput.wheels`가 증가하고 local gram13 scroll이 멈추는지
재검증해야 한다.

취소 이벤트와 메시지 대기의 Win32 동작은 [MsgWaitForMultipleObjectsEx 공식 문서](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-msgwaitformultipleobjectsex)를 따른다.

## 대안

- `GetRawInputBuffer` 반복 횟수만 제한: starvation 가능성을 낮출 뿐 hook과 high-frequency source의
  lifecycle 결합을 남겨 기각한다.
- 주기적으로 hook 재설치: 제거 여부를 알 수 없고 정상 callback 중복과 입력 손실 구간을 만들어 기각한다.
- Raw Input wheel로 다시 통합: gram13 Precision Touchpad가 Raw Input wheel을 제공하지 않은 기존 증거
  때문에 기각한다.
