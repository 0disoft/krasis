# System Boundary

- 상태: Active
- 제품 범위: 같은 사용자가 소유한 Windows 노트북 두 대의 안전한 입력 전환
- 기준: ADR 0001, ADR 0002, `docs/product/02-spec.md`

## 이 저장소가 소유하는 것

Krasis는 device trust record, authenticated peer connection, operational session, control lease,
ordered input stream, pressed-input recovery와 Windows input adapter를 소유한다. Phase 0에서는
`krasis-lab`, Win32 launcher, crash guard, aggregate evidence schema와 probe-kit도 이 저장소가 소유한다.

`internal/control`은 순수 상태와 identity·sequence guard, `internal/application.SessionRuntime`은 한 peer
session의 mutable state 직렬화, `internal/peer`는 lease·input·heartbeat orchestration,
`internal/transport`는 bounded framed wire contract, `internal/wininput`은 OS hook와 injection,
`internal/agent`는 connection과 lifecycle owner다. 저장소의 docs와 accepted ADR은 이 경계의 source다.

## 소유하지 않는 것

현재 저장소는 cloud account, internet relay, 원격 telemetry upload, clipboard·file·screen·audio 공유,
세 대 이상 topology, 다중 monitor topology, 비-Windows adapter, 자동 update와 release backend를
소유하지 않는다. Windows secure desktop, 로그인 화면과 높은 integrity process에 대한 입력 보장도
MVP 범위가 아니다.

Wails v3 + Svelte UI, 정식 installer, signing pipeline과 production tray agent는 목표 경계지만 아직
구현물이 아니다. 현재 `krasis-launcher.exe`는 Phase 0 probe kit를 운용하는 Win32 도구이며 제품 UI로
간주하지 않는다.

## Runtime Flow

1. source Windows adapter가 physical input과 configured screen edge를 관찰한다.
2. boundary intent가 새 session과 epoch를 만들고 destination에 lease request를 보낸다.
3. destination은 trust와 capability를 확인하고 inbound state를 `INBOUND_PENDING`으로 prepare한다.
4. source activation이 성공한 뒤에만 destination이 commit하고 양쪽이 active ownership으로 전환한다.
5. source runtime이 sequence를 발급하고 destination runtime이 exact session, epoch와 next sequence를
   승인한 payload만 adapter에 전달한다.
6. local return, peer revoke, timeout, disconnect, lifecycle event, injection failure 또는 identity
   불일치는 current lease를 폐기하고 `ReleaseAll`을 실행한다.
7. pressed token이 모두 해제된 뒤에만 session과 lease를 지우고 `LOCAL`로 복귀한다. 재연결은 새 identity와
   sequence 1에서 시작한다.

상세 write ordering과 failure compensation은 `docs/architecture/02-runtime-flow.md`와 ADR 0003부터
ADR 0012까지가 소유한다.

## Process and Data Boundaries

agent와 crash guard는 같은 사용자 권한으로 실행하며 guard는 상속된 anonymous pipe 외의 listener를
열지 않는다. launcher는 executable 옆 strict config와 사용자가 지정한 local report·credential
directory만 조립한다. future UI는 agent 상태를 투영하고 명령을 요청할 뿐 control lease, hook,
network connection과 `PressedState`를 직접 소유하지 않는다.

persistent product data는 현재 local trust store뿐이다. launcher config와 Phase 0 report는 실험용 local
data, runtime session·lease·sequence·pressed state는 process memory다. raw key text, pointer trajectory,
window metadata와 credential은 report나 일반 log 경계를 넘지 않는다.

## 품질 불변조건

- destination prepare 전 source input suppression 금지
- 한 runtime의 active lease는 하나
- unknown 또는 stale identity와 sequence 실행 금지
- uncertain delivery 재생 금지
- cleanup 실패를 성공이나 `LOCAL`로 기록 금지
- UI 종료가 agent cleanup을 막는 구조 금지
- generated output, cache와 report를 source of truth로 사용 금지

## 아직 결정하지 않은 경계

| 결정 | 책임 | blocker | 결정 trigger |
| --- | --- | --- | --- |
| production Windows version과 architecture floor | repository maintainers | 네 대 Phase 0 compatibility evidence 미완료 | #8 Phase 0 gate 종료 |
| production data directory와 installer scope | repository maintainers | 정식 resident agent와 signing layout 미구현 | #13과 #16 설계 확정 |
| UI와 agent 사이 local authenticated API | repository maintainers | production agent command surface 미확정 | #13 완료 뒤 #15 착수 |
| heartbeat와 peer timeout 제품 기본값 | repository maintainers | 실제 pair congestion·sleep 장시간 evidence 부족 | #7 long-run evidence 검토 |
| public LAN exposure policy와 firewall automation | repository maintainers | installer와 per-peer network policy 미구현 | #16 packaging 설계 |
