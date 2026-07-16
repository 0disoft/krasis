# ADR 0022: Precision Touchpad 전용 pointer 표면을 별도 진단한다

- 상태: Accepted
- 책임자: cherr
- 범위: Phase 0 Windows 11 Precision Touchpad 진단
- 관계: ADR 0019와 0021의 hook·Raw Input 진단을 보완하며 production wheel owner는 변경하지 않는다

## 맥락

gram13에서 `agent-session/v12`를 실제 실행한 결과, remote active 구간의 pointer move `290`건과
button `4`건은 source hook에서 관찰되고 HP15에 전달됐다. 같은 구간에 두 손가락 vertical/horizontal
scroll은 gram13의 local window에서 동작했지만 `sourcePipeline.wheel`의 `hookObserved`,
`synthesizedObserved`, `rawObserved`가 모두 `0`이었다. destination wheel도 `0`이었다.

따라서 이 재현은 network, source queue, destination injection 전체가 죽은 경우가 아니다. low-level
mouse hook도 move와 button callback을 계속 받고 있었다. 현재 관찰 경계 바깥에 Precision Touchpad
gesture가 노출되는지 먼저 확인하지 않고 mapper나 queue를 다시 바꾸면 원인을 건드리지 않은 채 schema만
늘어나게 된다.

Windows 11의 `RegisterTouchpadCapableWindow`는 등록된 top-level window가 두 손가락 gesture의
`WM_POINTER` 입력을 받도록 한다. pointer message를 `DefWindowProc`에 넘기면 legacy mouse wheel 변환이
발생할 수 있다. 이 API는 Windows 11 desktop app 전용이며 user32 ordinal `2689` fallback을 가진다.

## 결정

- lab-only `touchpad-surface` 명령을 추가한다. network, peer credential, hook suppression과
  `agent-session`은 사용하지 않는다.
- 명령은 전용 OS thread에 visible top-level window를 만들고 foreground로 올린 뒤
  `RegisterTouchpadCapableWindow(TRUE)`를 호출한다.
- `WM_POINTERDOWN`, `WM_POINTERUPDATE`, `WM_POINTERUP`, enter/leave/capture-changed와 pointer
  vertical/horizontal wheel message를 집계한다. 각 message에서 `GetPointerType`을 즉시 호출해
  `PT_TOUCHPAD` 여부만 센다.
- 모든 pointer message는 집계 후 `DefWindowProc`에 넘긴다. legacy `WM_MOUSEWHEEL`과
  `WM_MOUSEHWHEEL`도 별도 aggregate로 세되, 외장 mouse인지 `DefWindowProc` 변환인지 인과를 붙이지 않는다.
- report에는 pointer ID, 좌표, delta, timestamp, device path, window handle과 원시 message를 저장하지
  않는다.
- 실행 중 foreground를 잃거나 `PT_TOUCHPAD` pointer가 한 건도 확인되지 않으면 통과시키지 않는다.
  legacy wheel만 관찰된 결과는 원인 분석 자료로 보존하되 성공으로 분류하지 않는다.
- cleanup은 touchpad 등록 해제, window 파괴, class 해제 순서다. window close, Escape와 context 취소는
  bounded partial report를 남긴다.
- 이 진단 결과만으로 production wheel payload owner를 `WM_POINTER`로 이전하지 않는다. production
  경로 변경에는 active lease, local suppression, 중복 방지, 순서, queue overflow와 destination
  injection을 함께 검증하는 후속 ADR이 필요하다.

## 결과

`POINTER_ONLY` 또는 `POINTER_AND_LEGACY`이고 `touchpadConfirmed>0`이면 gram13 두 손가락 gesture가
현재 hook·Raw Input 경계 바깥의 Precision Touchpad pointer 표면에 존재한다는 다음 구현 근거를 얻는다.
`LEGACY_ONLY`는 legacy message가 보였다는 뜻일 뿐 해당 message의 장치나 변환 경로를 증명하지 않는다.
`NEITHER`는 창이 foreground였고 사용자가 실제 gesture를 수행했는지 다시 확인한 뒤 API/driver 경계를
재조사해야 한다.

이 명령은 visible window를 의도적으로 foreground로 만든다. 일반 작업 중 실행하면 focus를 빼앗을 수
있으므로 저장된 빈 테스트 환경에서만 사용한다. Windows 10이나 API export가 없는 build에서는
unsupported exit로 종료한다.

## 대안

- `WH_MOUSE_LL` lifecycle을 다시 변경: 같은 실행에서 move와 button callback이 살아 있어 wheel만 없는
  사실을 설명하지 못해 보류한다.
- Raw Input wheel을 즉시 production owner로 승격: active 실행에서 `rawObserved=0`이었으므로 기각한다.
- Deskflow 구현을 그대로 복제: Deskflow도 기본 Windows 경로는 low-level mouse hook의 wheel message에
  의존하며 현재 gram13의 누락 표면을 설명하지 못해 기각한다.
- pointer ID, 좌표와 delta를 원시 저장: 진단에 불필요하고 개인정보·입력 궤적 노출만 늘려 기각한다.

## 검증 gate

- gram13에서 visible test window를 15초 동안 foreground로 유지하고 vertical/horizontal 두 손가락
  scroll을 각각 수행한다.
- `api.registerAvailable`, `api.pointerTypeAvailable`, `api.registered`, `window.created`,
  `window.foregroundObserved`가 모두 `true`이고 `window.deactivations=0`인지 확인한다.
- `pointer.touchpadConfirmed>0`, `completed=true`, `passed=true`와 빈 `failureCodes`를 요구한다.
- 같은 artifact의 단위 테스트는 pointer type 집계, legacy-only 실패, foreground loss, unsupported API,
  no-overwrite와 aggregate privacy를 검증한다. hosted CI에서 실제 touchpad 입력이나 foreground 획득을
  성공 조건으로 요구하지 않는다.

## 참고

- [RegisterTouchpadCapableWindow](https://learn.microsoft.com/en-us/windows/win32/input-precisiontouchpad/registertouchpadcapable)
- [Precision Touchpad programming guide](https://learn.microsoft.com/en-us/windows/win32/input-precisiontouchpad/precision-touchpad-guide)
