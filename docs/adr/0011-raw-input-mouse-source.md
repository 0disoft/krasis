# ADR 0011: Windows mouse source는 Raw Input packet을 사용한다

- 상태: Accepted
- 책임자: cherr
- 범위: Windows `SourceHook`의 mouse 입력 수집과 로컬 억제
- 후속 결정: vertical/horizontal wheel 소유권은 ADR 0013이 대체한다

## 맥락

기존 source mapper는 `WH_MOUSE_LL`의 `MSLLHOOKSTRUCT.pt` 화면 좌표 차이를 상대 이동량으로
사용했다. 화면 경계에서는 cursor 좌표가 고정되거나 방향이 뒤섞여, 물리 장치는 계속 바깥으로
움직이는데 원격에는 작은 이동, 반대 방향 이동 또는 zero delta가 전달됐다.

2026-07-15 S-A/HP15의 오른쪽 경계 shadow에서 Raw Input은 edge 바깥 방향 packet을 884개
관찰했지만 hook 좌표는 바깥 방향 368개, 안쪽 방향 384개, X 변화 없음 178개로 분류됐다.
S-B/gram13 Precision Touchpad의 왼쪽 경계에서는 Raw Input 바깥 방향 625개와 안쪽 방향 0개였지만
hook 좌표는 바깥 방향 251개, 안쪽 방향 278개, X 변화 없음 96개였다. S-B의 675개 Raw Input
packet은 모두 zero device handle이었으므로 `hDevice == 0`을 결함이나 합성 입력으로 취급할 수 없다.

## 결정

- Windows mouse의 relative move, left/right/middle/X1/X2 button과 vertical/horizontal wheel은
  Raw Input `RAWMOUSE` packet 하나에서 정규화한다.
- 한 packet 안의 action 순서는 move, left/right/middle/X1/X2 down/up, vertical wheel,
  horizontal wheel이다. mouse action을 hook과 Raw Input 두 경로에서 중복 생성하지 않는다.
- `WH_MOUSE_LL`은 최신 screen X 좌표 관찰, Windows injected flag와 Krasis tag 판별,
  active outbound lease의 로컬 mouse event 억제에만 사용한다. hook 화면 좌표 차이는 원격
  pointer delta로 사용하지 않는다.
- keyboard는 기존 `WH_KEYBOARD_LL` scan-code 경로를 유지한다.
- source 전용 고정 OS thread가 message-only window, Raw Input mouse 등록과 두 low-level hook을
  함께 소유한다. process에서 mouse TLC를 등록하는 다른 Krasis component와 process-global
  hook gate를 공유한다.
- `GetMessage`가 꺼낸 현재 `WM_INPUT`은 `GetRawInputData`로 읽고, 그 사이 누적된 packet은
  8-byte aligned 64KiB 재사용 buffer의 `GetRawInputBuffer` loop로 모두 drain한다.
- packet당 goroutine, 동적 slice와 buffer allocation을 만들지 않는다. 각 normalized action은
  기존 bounded `SourceSession` queue에 packet 순서대로 non-blocking offer한다.
- relative packet의 `hDevice == 0`은 Precision Touchpad를 포함한 정상 입력으로 허용한다.
  known Krasis/self-test extra-info는 재전송하지 않는다.
- absolute mouse movement는 Phase 0 source에서 전달하지 않는다. 같은 packet의 지원 button과
  wheel은 계속 처리한다.
- Raw Input 등록·read·parse, hook 설치 또는 message loop가 실패하면 component를 실패시키고
  session cleanup으로 수렴한다. queue overflow도 기존과 같이 active generation을 즉시 폐기한다.

## 결과

원격 relative movement가 화면 경계의 절대 좌표 clipping과 Windows pointer acceleration에
의존하지 않는다. 같은 mouse packet의 move·button·wheel 순서도 한 source가 소유한다.
1000Hz 장치에서 현재 packet 뒤에 쌓인 입력은 batch로 drain하며 buffer는 session 동안 재사용한다.

Raw Input packet 수신과 low-level hook 억제는 하나의 원자적 Windows API가 아니다. active lease에서
물리 mouse hook event를 억제한 직후 Raw Input read나 queue가 실패하면 해당 event 하나는 로컬과
원격 모두에서 사라질 수 있다. 이 경우 전달을 추측하거나 재생하지 않고 source generation과 lease를
폐기해 suppression을 해제한다. 장기적으로 driver 없이 이 간극을 제거할 수 있다고 가정하지 않는다.

Raw Input과 keyboard hook 사이의 전역 inter-device 순서는 별도 timestamp 상관관계를 만들지 않는다.
같은 mouse packet 내부 순서만 보존한다. absolute pointing device, swapped physical buttons, 1000Hz
지속 부하의 queue 여유와 실제 end-to-end latency는 대상 장비 revalidation 전까지 제품 통과로
간주하지 않는다.

## 대안

- hook 화면 좌표 delta 유지: 두 실제 장비에서 방향 반전과 edge loss가 재현돼 기각한다.
- pointer만 Raw Input, button·wheel은 hook 유지: 같은 mouse의 move와 click 순서가 두 채널 사이에서
  뒤집힐 수 있어 기각한다.
- Raw Input keyboard까지 함께 전환: 현재 pointer 결함 범위를 넘고 keyboard hook은 실제 장비에서
  지연 없이 동작했으므로 후속 필요성이 생길 때 별도 결정한다.
- kernel driver: Phase 0 user-session 범위를 넘고 현재 증거로 필요성이 입증되지 않아 기각한다.
