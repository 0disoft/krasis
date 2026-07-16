# ADR 0019: production wheel 소유권 전에 입력 표면을 단일 장비에서 계측한다

- 상태: Accepted
- 책임자: cherr
- 범위: Phase 0 Windows wheel 진단과 production 소유권 재검토 gate
- 대체 관계: ADR 0013, 0017과 0018의 Raw Input wheel 부재라는 미검증 증거 결론을 대체한다

## 맥락

gram13의 두 손가락 scroll은 local 화면에 적용됐고 여러 `agent-session` report의 hook·source·destination
wheel count는 0이었다. 하지만 production Raw mapper가 `RI_MOUSE_WHEEL`과 `RI_MOUSE_HWHEEL`을
의도적으로 무시했으므로 그 0은 Raw Input wheel flag 부재를 계측한 값이 아니다. 기존 ADR들이 이 값을
근거로 Raw Input 후보를 기각한 부분은 증거보다 앞서 나갔다.

## 결정

- `wheel-surface`가 한 장비에서 low-level hook과 Raw Input wheel aggregate를 동시에 수집한다.
- 계약은 `krasis-lab wheel-surface --device H-A|H-B|S-A|S-B --duration DURATION --output PATH.json`,
  schema는 `krasis.lab.wheel-surface/v1`이다. network, credential, suppression과 두 번째 노트북은 없다.
- hook은 physical move/button과 physical/tag 없는 synthesized vertical·horizontal wheel을 분리한다.
  Raw Input은 packet, zero-device, tagged-injected, wheel flag와 signed delta total·magnitude를 집계한다.
- 시작과 종료에 `MOUSEEVENTF_MOVE|MOUSEEVENTF_MOVE_NOCOALESCE` tagged 상대 move `+1/-1` pair를
  넣는다. 총 네 `WM_MOUSEMOVE`를 hook에서 관찰·차단하며 wheel event를 만들지 않아 Raw wheel evidence를
  오염시키지 않는다.
- pass에는 sentinel 네 건의 삽입·관찰·차단과 non-sentinel physical move 또는 button down/up 중 하나
  이상이 필요하다. physical wheel 자체는 gate가 아니며 실제 interaction이 있는 `NEITHER`도 유효하다.
- `observedPath`는 `HOOK_AND_RAW`, `HOOK_ONLY`, `RAW_ONLY`, `NEITHER`다. gram13 touchpad와 일반 mouse
  대조 결과 전에는 production wheel owner를 확정하지 않는다.
- output은 기존 file을 덮어쓰지 않고 좌표, 개별 delta, trajectory, 장치·창 identity를 저장하지 않는다.

## 결과

wheel ingress를 transport와 lease에서 떼어낸다. `RAW_ONLY`면 hook-owned 결정을 재검토하고,
`NEITHER`면 Precision Touchpad의 pointer/gesture 표면을 조사한다. `HOOK_ONLY`도 active session에서
callback이 사라지는 lifecycle 문제를 별도로 남긴다. `HOOK_AND_RAW`는 owner와 dedupe를 자동 결정하지 않는다.

hook이 없으면 sentinel cursor move가 local desktop에 순간 노출될 수 있다. pair 순합은 0이지만 완전히
무해하거나 보이지 않는다고 보장하지 않으므로 중요한 작업을 저장하고 빈 테스트 화면에서 실행한다.

## 대안

- `agent-session` 반복: 계측하지 않은 Raw wheel을 계속 0으로 오해하므로 기각한다.
- hook-owned 또는 Raw-owned를 즉시 확정: 실제 대조 report가 없어 둘 다 기각한다.
- `WM_POINTER*`를 즉시 production에 추가: 입력 표면 확인과 권한·foreground 설계가 먼저다.

## 검증 gate

- gram13 touchpad와 일반 mouse를 서로 다른 새 output으로 수집한다.
- 두 report 모두 completed, sentinel health와 physical interaction gate를 통과해야 한다.
- 결과에 따라 production owner, suppression과 dedupe를 후속 ADR로 확정한다.
