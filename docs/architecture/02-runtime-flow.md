# 런타임 흐름

- 상태: Draft

## 시작

1. 단일 agent 인스턴스가 로컬 신원, 신뢰 peer와 설정을 읽는다.
2. Windows 입력 adapter와 로컬 projection API를 시작한다.
3. 연결이 없거나 peer 인증이 끝나지 않으면 `LOCAL`을 유지한다.

## 경계 전환

1. Windows adapter가 경계 의도를 감지한다.
2. Application이 destination 연결, 세션과 제어권 가능 여부를 확인한다.
3. destination이 수신 준비를 확인한다.
4. source가 `OUTBOUND_ACTIVE`, destination이 `INBOUND_ACTIVE`가 된 뒤 입력 전달을 시작한다.
5. 반대 경계 전환은 같은 절차로 새 제어권을 만든다.

상태 변경은 `internal/control` transition table에서만 결정한다. table이 반환한 effect는 Application이 순서대로 실행하고 성공 또는 실패 event를 다시 상태 머신에 넣는다. 현재 구현은 순수 transition core, authenticated peer transport와 `SendInput` 기반 Win32 destination adapter까지 포함한다. source-side 경계 감지와 물리 입력 억제는 후속 단계다.

활성 inbound lease에서 받은 입력은 실행 전에 `InputStream` gate를 통과한다. session, epoch 또는 정확한 다음 sequence가 하나라도 맞지 않으면 해당 payload를 실행하지 않고 `FAULT_DETECTED` cleanup으로 넘긴다. stream과 lease의 session이 서로 다르거나 non-local 상태에서 lease 방향과 맞지 않는 입력을 받은 경우도 동일하게 fail-safe 처리한다. 재연결은 새 session과 epoch, sequence `1`에서 시작한다.

Application coordinator는 transition이 반환한 effect를 순서대로 실행한다. 한 effect가 실패해도 같은 cleanup 묶음의 나머지 effect는 계속 시도한다. activation 또는 cleanup effect 실패는 `FAULT_DETECTED`로 환류하고, `ReleaseAll`이 성공하면 `RELEASE_ALL_SUCCEEDED`를 적용해 `LOCAL`까지 수렴한다. `RESUME_LOCAL`이 실패하면 같은 호출에서 무한 반복하지 않고 `RELEASE_ALL_FAILED`를 적용해 `FAULT`와 복구 상태를 유지한다.

heartbeat deadline은 caller가 제공한다. 만료된 현재-session liveness는 위 coordinator에 `FAULT_DETECTED`를 전달하며, 아직 살아 있는 session에는 state나 effect를 만들지 않는다. session 불일치, 시간이 뒤로 이동함, 잘못된 timeout처럼 liveness 판정을 신뢰할 수 없는 경우도 active lease를 유지하지 않고 같은 cleanup 경로로 보낸다.

## 장애와 종료

1. 추가 입력 억제와 전달을 중단한다.
2. 활성 lease를 폐기한다.
3. destination의 `PressedState`에 대해 `ReleaseAll`을 수행한다.
4. 입력 adapter와 연결을 닫는다.
5. 양쪽 상태를 `LOCAL`로 만든다.

순서를 확정할 수 없는 연결 복구에서는 미전달 입력을 재생하지 않는다. GUI 종료는 agent 종료와 같은 뜻이 아니며, agent 종료 요청은 위 안전 종료 절차를 반드시 거친다.

현재 Application 구현은 in-memory snapshot과 injected adapter port를 대상으로 한다. `SessionRuntime`은 lease transition, input sequence 승인, pressed-state 변경, 실제 key/button 주입과 주입 실패 cleanup을 하나의 직렬 임계구역에서 수행한다. heartbeat 관찰과 만료 판정도 같은 owner를 거친다.

정규화된 pointer move와 wheel action도 같은 sequence와 owner 경계를 통과한다. action 형식 오류나 adapter 주입 실패는 해당 payload를 재생하지 않고 session fault cleanup으로 수렴한다.

Win32 destination adapter는 keyboard scan-code down/up, left/right/middle/X1/X2 button down/up, relative pointer move와 vertical/horizontal wheel을 `SendInput`으로 주입한다. 두 wheel 축이 한 action에 있으면 vertical 다음 horizontal 순서의 한 batch로 보낸다. context가 이미 취소됐거나 Windows가 batch 일부만 삽입하면 성공으로 가장하지 않고 adapter 오류를 반환해 session cleanup을 시작한다. 합성 입력에는 Krasis tag를 넣지만 보안 판단은 tag 하나가 아니라 Windows injected flag와 현재 session 방향을 함께 사용해야 한다.

lab self-test의 tagged `+1/-1` 상대 이동은 두 injection이 hook으로 돌아오는지 검증하는 probe이며, Windows pointer acceleration과 pixel rounding 때문에 두 delta가 좌표상 정확히 상쇄된다고 가정하지 않는다. 두 event 뒤 좌표가 남으면 `SetCursorPos`로 시작 좌표를 복구하고 그 결과까지 확인한다. 이 복구는 명시적인 bounded self-test에만 사용하며 제품의 일반 pointer 전달 경로에는 사용하지 않는다.

source mapper는 physical keyboard hook의 scan code와 extended flag를 canonical key code로 바꾸고, mouse hook의 좌표 변화·button·signed wheel 값을 normalized action으로 바꾼다. injected flag 또는 Krasis tag가 있는 keyboard/mouse event는 다시 전송하지 않는다. injected mouse move도 다음 physical delta가 튀지 않도록 좌표 baseline만 갱신한다.

source session은 hook callback과 network sender 사이에 고정 용량 queue를 둔다. callback은 현재 outbound generation에 action을 non-blocking enqueue한 경우에만 해당 물리 event를 억제할 수 있다. queue overflow, invalid mapping 또는 sender 실패는 suppression을 즉시 끄고 generation을 폐기하며, worker가 runtime fault cleanup을 수행한다. 폐기된 generation의 queued action은 원격으로 보내지 않는다. queue 용량은 설정 가능한 `1..65536` 범위이며 실제 기본값과 성능 예산은 source hook 통합 측정 뒤 확정한다.

`SourceHook`은 별도 고정 OS thread에 `WH_KEYBOARD_LL`과 `WH_MOUSE_LL`을 설치한다. callback은 mapper, `BoundaryIntentEmitter.Observe`, `SourceSession.Offer`만 호출하고, offer가 성공한 physical event만 non-zero hook result로 억제한다. boundary intent handoff는 용량 1 queue에 non-blocking으로 수행하며 이미 intent가 대기 중이면 새 intent를 버리고 해당 physical event는 로컬 처리한다. inactive, injected, mapping error 또는 source queue overflow event도 `CallNextHookEx`로 로컬 처리에 넘긴다. 제품 source hook와 lab capture는 process-global hook gate를 공유하므로 같은 process에서 동시에 설치되지 않는다.

`OutboundController`는 boundary orchestrator와 control dispatcher가 공유하는 source-side lifecycle owner다. boundary intent에서는 `LeaseRequest`만 보내고 source suppression을 켜지 않는다. 정확한 session·epoch의 `LeaseGrant`가 runtime을 `OUTBOUND_ACTIVE`로 만든 뒤에만 `SourceSession.Activate`를 호출한다. activation 실패, reject, revoke와 session failure에서는 suppression을 먼저 끄고 runtime cleanup으로 수렴한다.

`BoundaryOrchestrator`는 용량 1 intent queue를 순차 소비하고 각 새 outbound 요청에 `crypto/rand` 기반 128-bit session ID와 별도 128-bit epoch를 생성한다. runtime이 `LOCAL`이 아니거나 source suppression이 이미 활성화된 동안 도착한 intent는 현재 session을 건드리지 않고 버린다. entropy 생성 실패와 최초 request write 실패는 orchestrator error로 supervisor에 전달한다.

`LeaseRequest`는 source가 넘은 `LEFT` 또는 `RIGHT` edge를 포함한다. destination `CursorEntryPort`는 승인된 `RIGHT` request를 virtual desktop 왼쪽에서 1px 안쪽, `LEFT` request를 오른쪽에서 1px 안쪽, 세로 중앙에 배치한다. cursor 배치가 성공한 뒤에만 inbound runtime을 활성화하고 grant를 쓴다. authorization reject는 cursor를 움직이지 않으며 cursor 배치 실패는 inbound lease나 grant를 만들지 않고 session failure로 처리한다.

destination input adapter는 현재 inbound session·epoch로 return detector를 arm한다. source `RIGHT` 진입은 destination 왼쪽 끝, source `LEFT` 진입은 destination 오른쪽 끝에서 return intent를 non-blocking으로 한 번만 발행한다. 오래된 queued intent는 새 session arm에서 폐기하고 runtime은 현재 inbound session·epoch가 정확히 일치하며 pressed input이 없을 때만 local revoke를 승인한다. 승인된 revoke는 destination을 먼저 `LOCAL`로 cleanup한 뒤 exact session·lease·epoch의 `LeaseRevoke`를 쓰며 source는 이를 받은 뒤 suppression을 해제한다. intent와 button-down의 경합은 session failure가 아니라 detector rearm으로 처리한다.

source `RunSession`의 EOF, cancellation, malformed frame과 dispatcher failure는 outbound lifecycle의 `Fail`을 통과해야 한다. 이 경로는 source suppression을 먼저 해제한 뒤 bounded runtime cleanup을 수행한다. runtime만 `LOCAL`로 돌리고 hook suppression을 남기는 종료 경로는 허용하지 않는다.

runtime은 port 호출이 끝날 때까지 owner lock을 유지한다. 따라서 effect와 input port는 같은 runtime API로 재진입하지 않아야 하며, 느린 I/O를 직접 수행하는 대신 adapter 내부의 명확히 제한된 호출만 제공해야 한다. transport와 Windows adapter는 이 runtime 경계를 통해서만 event와 input을 전달한다.

durable state commit, optimistic version compare-and-swap, transition history, command idempotency와 별도 crash guard는 아직 후속 경계다. 따라서 이 단계의 테스트는 process 내부 수렴을 증명하지만 process가 죽은 뒤의 복구를 증명하지 않는다.

## Peer transport

MVP transport framing은 `docs/adr/0003-ordered-peer-transport.md`를 따른다. 하나의 ordered stream에서 bounded frame을 순서대로 읽고, transport는 control, input, heartbeat kind와 payload 크기까지만 검증한다. payload decode와 runtime 호출은 인증된 connection을 소유한 adapter 책임이다.

server와 client TLS config는 TLS 1.3만 허용하고 `krasis/1` ALPN을 사용한다. server는 신뢰 root로 검증된 client certificate를 요구하고 client도 명시적인 expected server name과 신뢰 root로 server certificate를 검증한다. system root fallback, insecure skip verify와 익명 연결은 사용하지 않는다.

`PeerConnection`은 mutual TLS와 ALPN 검증이 완료된 뒤에만 bounded frame reader/writer를 노출한다. writer 호출은 connection 안에서 직렬화해 여러 producer의 frame bytes가 섞이지 않게 한다. input dispatcher는 payload decode가 실패하면 활성 `SessionRuntime`을 fault cleanup하고, router가 잘못 전달한 non-input frame은 runtime state를 건드리지 않고 거부한다.

## 상주 agent session 조립

`internal/agent.RunSession`은 인증이 끝난 peer connection 하나에 다음 lifecycle을 한꺼번에 묶는다.

- `SourceSession` worker를 먼저 시작하고 ready 이후에만 low-level source hook과 boundary orchestrator를 시작한다.
- 같은 `SessionRuntime`과 connection을 outbound controller, inbound lease responder, return controller가 공유한다.
- 한 peer가 경계를 넘으면 source suppression은 exact lease grant 뒤에만 활성화된다.
- 반대 peer가 경계를 넘으면 같은 process가 destination input adapter와 return controller로 동작한다.
- source worker, hook, boundary orchestrator, return controller, peer reader 중 하나가 비정상 종료하면 공통 context를 취소하고 나머지를 모두 회수한다.
- 종료 결과가 `LOCAL`이 아니면 정상 종료로 보고하지 않는다.

`internal/agent.RunSupervisor`는 이 session 조립 경계 바깥에서 listen 또는 dial connection source를 반복 실행한다. supervisor 상태는 `STOPPED`, `ACQUIRING`, `ACTIVE`, `BACKOFF`이며 transition table에 없는 전이는 거부한다. connection 획득 실패와 active session 실패는 서로 다른 failure kind로 관찰되지만 주소, certificate, session ID와 원시 오류 문자열은 snapshot에 넣지 않는다.

재시도 간격은 첫 250ms부터 두 배로 증가하고 30초에서 제한된다. session이 30초 이상 유지된 뒤 실패하면 다음 시도는 첫 간격으로 되돌아간다. 이 값은 현재 단일 peer 실험 기본값이며 실제 장비 측정 전 제품 SLO가 아니다. jitter는 한 listener와 한 connector만 있는 MVP topology에서는 아직 적용하지 않는다.

supervisor는 session 종료 결과가 `LOCAL`일 때만 새 connection을 얻는다. cleanup이 끝나지 않아 `FAULT` 또는 다른 상태에 남으면 reconnect로 덮지 않고 supervisor 자체를 실패시킨다. context 취소는 listener와 active connection을 닫고 `STOPPED`로 수렴한다.

`ListenSource`와 `DialSource`는 TCP와 TLS 세부사항을 supervisor core 밖에 둔다. 둘 다 `PeerConnection` 생성 시 TLS 1.3 mTLS와 ALPN 인증을 통과해야 connection을 application에 넘긴다. credential storage, 단일 instance, tray와 최종 agent executable은 아직 바깥 책임이다.

control payload는 wire 전용 `LeaseRequest`, `LeaseGrant`, `LeaseReject`, `LeaseRevoke` 명령만 허용한다. 모든 명령은 session과 epoch를 포함하고, grant와 revoke만 lease를 포함한다. `FAULT_DETECTED`, `RELEASE_ALL_SUCCEEDED` 같은 내부 상태 머신 event는 wire command가 아니며 peer가 직렬화하거나 주입할 수 없다.

인증된 peer session read loop는 control, input, heartbeat frame을 한 ordered stream에서 순차 dispatch한다. payload 오류, 잘못된 방향, handler 실패, EOF와 context 취소는 모두 connection을 닫고 runtime의 fault cleanup을 호출한다. EOF는 정상적인 제어 종료로 취급하지 않으며 명시적인 revoke 없이 연결이 끝난 경우에도 `ReleaseAll` 후 `LOCAL`로 수렴해야 한다.

wire 송신은 상태 머신 effect executor가 아니라 peer session orchestrator가 runtime 전이 성공 뒤 수행한다. `BeginOutboundSession`은 먼저 `PENDING`을 확정한 뒤 `LeaseRequest`를 쓰며, write 실패 시 pending session을 폐기한다. destination의 `LeaseResponder`는 주입된 authorization policy 결과에 따라 reject를 보내거나 inbound runtime을 활성화한 뒤 grant를 보낸다. grant write가 불확실하면 destination도 즉시 cleanup한다. input write는 runtime이 sequence를 예약한 뒤 수행하며 write 결과가 불확실하면 sequence를 재사용하지 않고 session 전체를 폐기한다.
