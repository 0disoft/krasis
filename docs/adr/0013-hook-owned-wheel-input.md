# ADR 0013: Windows wheel 입력은 low-level mouse hook이 소유한다

- 상태: Accepted
- 책임자: cherr
- 범위: Windows `SourceHook`의 vertical/horizontal wheel 수집과 억제
- 대체 관계: ADR 0011의 Raw Input wheel 소유권 결정을 대체한다

## 맥락

ADR 0011은 relative move, button과 wheel을 모두 `RAWMOUSE` packet에서 정규화해 같은 물리
mouse의 중복과 순서 역전을 막았다. 그러나 2026-07-15 gram13 Precision Touchpad의 실제
두 손가락 스크롤은 low-level mouse hook `capture`에서 physical wheel 505회로 관찰됐지만,
같은 build의 active agent session에서 `sourceInput.wheels=0`이었다. HP15 destination도
`destinationInput.wheels=0`이었고 pointer 1132회와 button down/up 2/2는 정상 수신했다.
스크롤은 transport나 `SendInput`에서 실패한 것이 아니라 Raw Input source에 나타나지 않았다.

## 결정

- relative pointer move와 left/right/middle/X1/X2 button은 계속 Raw Input이 소유한다.
- vertical `WM_MOUSEWHEEL`과 horizontal `WM_MOUSEHWHEEL`은 `WH_MOUSE_LL` callback이 소유한다.
- hook의 `MSLLHOOKSTRUCT.mouseData` high word를 signed 16-bit delta로 정규화한다.
- physical wheel action이 현재 outbound generation의 bounded `SourceSession` queue에 성공적으로
  들어간 경우에만 hook callback이 non-zero를 반환해 로컬 wheel을 억제한다.
- inactive session, zero delta, unknown message, Windows injected flag, Krasis tag 또는 self-test
  tag가 있는 wheel은 원격으로 보내거나 억제하지 않는다.
- Raw Input mapper는 vertical/horizontal wheel flag를 의도적으로 무시한다. 일반 mouse가 hook과
  Raw Input 양쪽에 wheel을 노출해도 원격 wheel은 hook에서 정확히 한 번만 생성된다.
- high-frequency mouse move callback은 wheel mapper와 panic boundary를 타지 않는다. wheel message만
  mapper를 호출하며 callback 안에서 blocking I/O나 allocation을 추가하지 않는다.

## 결과

Precision Touchpad 두 손가락 스크롤과 일반 mouse wheel이 같은 low-level hook 경로를 사용한다.
Raw Input move·button과 hook wheel 사이의 엄격한 전역 순서는 보장하지 않는다. wheel을 잃는 것보다
이 경계가 실제 Windows 입력 표면에 맞고, keyboard와 mouse가 이미 서로 다른 hook/Raw Input source를
사용하므로 ordered transport는 queue에 들어온 이후의 순서만 보존한다.

hook event 억제와 sender의 실제 network write는 원자적이지 않다. queue offer 뒤 sender가 실패하면
기존 source fault cleanup이 generation과 lease를 폐기한다. 전달 성공을 추측해 로컬 wheel을 재생하지
않는다. 실제 gram13→HP15 wheel 전달과 일반 mouse의 단일 scroll 여부는 새 build의 두 장비 재검증 전까지
통과로 간주하지 않는다.

## 대안

- Raw Input wheel 유지: Precision Touchpad의 실제 스크롤이 505회 모두 누락돼 기각한다.
- hook과 Raw Input wheel을 모두 보내고 시간 기반 dedupe: 지연, timer와 장치별 heuristic을 만들고
  어느 event가 원본인지 증명할 수 없어 기각한다.
- Precision Touchpad만 hook fallback: hook event에 신뢰할 수 있는 물리 장치 identity가 없어 일반
  mouse와 안정적으로 구분할 수 없으므로 기각한다.
