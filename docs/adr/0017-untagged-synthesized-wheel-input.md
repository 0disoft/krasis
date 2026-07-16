# ADR 0017: 태그 없는 Windows 합성 wheel을 source 입력으로 인정한다

- 상태: Accepted
- 책임자: cherr
- 범위: Windows `WH_MOUSE_LL` wheel 분류와 Phase 0 source pipeline evidence
- 대체 관계: ADR 0013의 모든 `LLMHF_INJECTED` wheel 제외 결정과 ADR 0014의 hook 관찰 집계를 대체한다

## 맥락

2026-07-15 gram13에서 HP15로 제어권을 넘긴 v10 재실험은 pointer 782회와 button down/up 각 9회를
양쪽 report에서 정확히 일치시켰고 HP15의 `supervisor.activeCount=1`, `sessionFailures=0`으로 장기
connection도 안정화했다. 그러나 gram13의 Precision Touchpad 두 손가락 scroll은 gram13 로컬 화면에
적용됐고 `sourcePipeline.wheel`의 `hookObserved`, `mapped`, queue와 `sent`가 모두 0이었다.

Windows `MSLLHOOKSTRUCT.flags`의 `LLMHF_INJECTED`는 어떤 process가 합성한 event인지 나타낼 뿐
Krasis가 주입한 event의 identity가 아니다. Krasis의 `SendInput`은 별도 `dwExtraInfo` tag를 이미
사용한다. 두 신호를 같은 것으로 취급하면 Windows 또는 touchpad stack이 합성한 wheel까지 source
callback 입구에서 폐기한다.

## 결정

- Krasis `dwExtraInfo` tag와 self-test tag가 있는 wheel은 계속 원격 전송과 억제 대상에서 제외한다.
- tag가 없고 `LLMHF_INJECTED`만 설정된 vertical/horizontal wheel은 source input으로 정규화한다.
- 해당 action이 현재 outbound generation의 bounded queue에 들어간 경우에만 로컬 wheel을 억제한다.
  inactive, invalid 또는 overflow 거절은 기존 fail-open·fault 경계를 유지한다.
- tag 없는 합성 wheel 관찰 수는 `synthesizedObserved`로 physical `hookObserved`와 분리한다.
- `krasis.lab.agent-session/v11`의 strict invariant는
  `mapped<=hookObserved+synthesizedObserved`를 overflow 없이 검사한다. 나머지 queue rejection과
  transport `sent` 합계 계약은 유지한다.
- strict checker는 `krasis.lab.agent-session-check/v8`, Phase 0 aggregate는
  `krasis.lab.phase0-check/v8`로 올린다.
- pointer와 button의 generic low-level hook suppression은 계속 모든 injected event를 제외한다.
  이 변경은 hook-owned wheel 경로에만 적용한다.

## 결과

원격 Krasis wheel은 stable tag로 feedback loop에서 제외하면서 Precision Touchpad driver나 Windows가
합성한 tag 없는 wheel을 전달할 수 있다. 다른 automation program이 만든 tag 없는 wheel도 active
lease 중에는 원격으로 전달될 수 있으므로 장비 실험에서는 Deskflow 같은 입력 공유 프로그램을 계속
종료해야 한다. report는 payload, delta, 좌표나 producer identity를 저장하지 않고 source 종류별 aggregate
count만 남긴다.

다음 실제 gram13 실험에서 `synthesizedObserved>0`, `mapped=queueAccepted=sent>0`, HP15의
`destinationInput.wheels>0`과 gram13 로컬 scroll 미발생을 함께 확인해야 한다. 일반 mouse wheel은
`hookObserved>0`, `synthesizedObserved=0`과 한 notch당 단일 적용을 별도로 확인한다.

## 대안

- 모든 `LLMHF_INJECTED` 유지 제외: 실제 local scroll과 zero pipeline 증거를 설명하지만 결함을 그대로
  남겨 기각한다.
- Raw Input wheel로 되돌림: 같은 gram13 Precision Touchpad가 Raw Input source에서 누락된 기존 실험
  때문에 기각한다.
- producer process를 식별해 allowlist: low-level hook 구조체가 안정적인 producer identity를 제공하지
  않아 기각한다.
