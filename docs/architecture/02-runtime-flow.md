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

runtime은 port 호출이 끝날 때까지 owner lock을 유지한다. 따라서 effect와 input port는 같은 runtime API로 재진입하지 않아야 하며, 느린 I/O를 직접 수행하는 대신 adapter 내부의 명확히 제한된 호출만 제공해야 한다. transport와 Windows adapter는 이 runtime 경계를 통해서만 event와 input을 전달한다.

durable state commit, optimistic version compare-and-swap, transition history, command idempotency와 별도 crash guard는 아직 후속 경계다. 따라서 이 단계의 테스트는 process 내부 수렴을 증명하지만 process가 죽은 뒤의 복구를 증명하지 않는다.

## Peer transport

MVP transport framing은 `docs/adr/0003-ordered-peer-transport.md`를 따른다. 하나의 ordered stream에서 bounded frame을 순서대로 읽고, transport는 control, input, heartbeat kind와 payload 크기까지만 검증한다. payload decode와 runtime 호출은 인증된 connection을 소유한 adapter 책임이다.

server와 client TLS config는 TLS 1.3만 허용하고 `krasis/1` ALPN을 사용한다. server는 신뢰 root로 검증된 client certificate를 요구하고 client도 명시적인 expected server name과 신뢰 root로 server certificate를 검증한다. system root fallback, insecure skip verify와 익명 연결은 사용하지 않는다.

`PeerConnection`은 mutual TLS와 ALPN 검증이 완료된 뒤에만 bounded frame reader/writer를 노출한다. writer 호출은 connection 안에서 직렬화해 여러 producer의 frame bytes가 섞이지 않게 한다. input dispatcher는 payload decode가 실패하면 활성 `SessionRuntime`을 fault cleanup하고, router가 잘못 전달한 non-input frame은 runtime state를 건드리지 않고 거부한다.

control payload는 wire 전용 `LeaseRequest`, `LeaseGrant`, `LeaseReject`, `LeaseRevoke` 명령만 허용한다. 모든 명령은 session과 epoch를 포함하고, grant와 revoke만 lease를 포함한다. `FAULT_DETECTED`, `RELEASE_ALL_SUCCEEDED` 같은 내부 상태 머신 event는 wire command가 아니며 peer가 직렬화하거나 주입할 수 없다.

인증된 peer session read loop는 control, input, heartbeat frame을 한 ordered stream에서 순차 dispatch한다. payload 오류, 잘못된 방향, handler 실패, EOF와 context 취소는 모두 connection을 닫고 runtime의 fault cleanup을 호출한다. EOF는 정상적인 제어 종료로 취급하지 않으며 명시적인 revoke 없이 연결이 끝난 경우에도 `ReleaseAll` 후 `LOCAL`로 수렴해야 한다.

wire 송신은 상태 머신 effect executor가 아니라 peer session orchestrator가 runtime 전이 성공 뒤 수행한다. `BeginOutboundSession`은 먼저 `PENDING`을 확정한 뒤 `LeaseRequest`를 쓰며, write 실패 시 pending session을 폐기한다. destination의 `LeaseResponder`는 주입된 authorization policy 결과에 따라 reject를 보내거나 inbound runtime을 활성화한 뒤 grant를 보낸다. grant write가 불확실하면 destination도 즉시 cleanup한다. input write는 runtime이 sequence를 예약한 뒤 수행하며 write 결과가 불확실하면 sequence를 재사용하지 않고 session 전체를 폐기한다.
