# Architecture Decisions

- 상태: Active
- 책임자: cherr

## 결정 목록

- `0001-initial-architecture-boundaries.md`: 상주 agent와 projection 분리
- `0002-contract-source-of-truth.md`: 계약 source of truth
- `0003-ordered-peer-transport.md`: 단일 ordered peer transport와 bounded frame
- `0004-phase-0-crash-guard.md`: Phase 0 별도 입력 crash guard와 ACK 순서
- `0005-semantic-session-write-order.md`: session 의미 순서와 heartbeat write coordinator
- `0006-persistent-trust-store.md`: peer별 durable trust, revocation과 atomic local store
- `0007-windows-agent-lifecycle-owner.md`: Windows 절전·session·display event의 fail-closed agent owner
- `0008-input-injection-certainty.md`: `SendInput` 실패 확실성과 journal·crash guard rollback 경계
- `0009-cursor-entry-compensation.md`: grant 전 cursor entry의 prepare·commit·conditional rollback
- `0010-input-failure-evidence.md`: 입력 주입 실패 확실성과 cursor rollback 결과의 aggregate evidence
- `0011-raw-input-mouse-source.md`: Raw Input mouse source와 low-level hook 억제 역할 분리
- `0012-one-way-agent-evidence.md`: 장비별 source/destination 집계와 단방향 pair evidence
- `0013-hook-owned-wheel-input.md`: Precision Touchpad를 포함한 hook-owned wheel 입력
- `0014-source-pipeline-failure-evidence.md`: wheel pipeline 단계와 stable session failure component
- `0015-return-boundary-hysteresis.md`: destination 복귀 히스테리시스와 폐기된 source generation 격리
- `0016-post-return-input-drain.md`: local return 뒤 exact late input drain과 retry 가능한 owned close 오류
- `0017-untagged-synthesized-wheel-input.md`: Krasis tag와 Windows 합성 wheel 입력 분리
- `0018-dedicated-low-level-hook-thread.md`: Raw Input과 분리된 low-level hook 전용 thread
- `0019-wheel-input-surface-diagnostic.md`: hook·Raw Input wheel 표면과 hook health를 분리하는 단일 장비 진단
- `0020-post-return-heartbeat-drain.md`: local return 뒤 retired inbound heartbeat drain
- `0021-active-session-wheel-surface-evidence.md`: hook-owned wheel을 유지한 active-session Raw/hook 진단 evidence
- `0022-touchpad-capable-window-surface-diagnostic.md`: Windows 11 Precision Touchpad pointer 표면의 foreground 진단
- `0023-active-touchpad-window-wheel-owner.md`: active outbound touchpad-capable window wheel owner와 focus 복원

새 결정은 `0000-template.md`를 복사해 다음 번호로 추가한다. 이미 Accepted인 결정의 의미를 바꿀 때는 기존 문서를 조용히 덮어쓰지 말고 새 ADR에서 대체 관계와 migration 영향을 기록한다.
