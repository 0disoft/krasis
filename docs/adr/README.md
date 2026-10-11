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
- `0024-control-flow-failure-attribution.md`: boundary handoff 단계와 실제 session failure origin의 aggregate evidence
- `0025-local-menu-boundary-admission.md`: local Windows menu loop의 source-side 전환 차단과 failure 우선순위
- `0026-phase-0-native-launcher.md`: 실시간 입력 경로 밖 native launcher와 안전 종료·정확한 report 복사
- `0027-local-interaction-admission-and-two-phase-entry.md`: drag·pressed input 차단과 commit/abort 기반 destination entry 확정
- `0028-bounded-foreground-thread-attach-fallback.md`: foreground 거절 때만 단일 retry와 즉시 detach를 수행하는 capture fallback
- `0029-bounded-focus-restore-thread-attach-fallback.md`: source focus 복원 거절 때만 단일 retry와 즉시 detach를 수행하는 대칭 fallback
- `0030-degraded-focus-restore-does-not-terminate-session.md`: capture foreground를 놓았다면 정확한 focus 복원 실패를 degraded로 유지
- `0031-peer-dispatch-and-input-queue-evidence.md`: peer dispatch, source queue 체류와 destination input disposition evidence
- `0032-live-trust-authorization-and-certificate-validity.md`: lease별 trust reload와 credential 유효기간 조기 검증
- `0033-hybrid-pre-auth-admission.md`: trusted-private listener와 pre-auth admission budget
- `0034-exact-active-lease-management-revocation.md`: exact active lease의 인증된 관리 종료
- `0035-product-identity-and-mutual-pairing.md`: protected product identity와 양쪽 confirmation receipt pairing
- `0036-resident-agent-and-local-api-ownership.md`: single owner, protected local API와 bounded shutdown
- `0037-product-cli-and-local-api-projection.md`: 제품 CLI, local API projection, config와 exit code

- `0069-pre-login-connection-owner.md`: 로그인 전 서비스 연결의 수명 관리 기반과 단계별 통합 계획
- `0071-applied-input-peer-progress.md`: 정상 처리한 현재 inbound 입력을 생존 근거로 반영하고 만료·실패 입력은 제외

새 결정은 `0000-template.md`를 복사해 다음 번호로 추가한다. 이미 Accepted인 결정의 의미를 바꿀 때는 기존 문서를 조용히 덮어쓰지 말고 새 ADR에서 대체 관계와 migration 영향을 기록한다.
