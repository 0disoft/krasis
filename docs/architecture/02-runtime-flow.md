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

상태 변경은 `internal/control` transition table에서만 결정한다. table이 반환한 effect는 Application이 순서대로 실행하고 성공 또는 실패 event를 다시 상태 머신에 넣는다. 현재 구현은 순수 transition core와 injected effect/input adapter port까지이며 실제 peer transport와 Win32 input adapter 연결은 후속 단계다.

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

runtime은 port 호출이 끝날 때까지 owner lock을 유지한다. 따라서 effect와 input port는 같은 runtime API로 재진입하지 않아야 하며, 느린 I/O를 직접 수행하는 대신 adapter 내부의 명확히 제한된 호출만 제공해야 한다. transport와 Windows adapter는 이 runtime 경계를 통해서만 event와 input을 전달한다.

durable state commit, optimistic version compare-and-swap, transition history, command idempotency와 별도 crash guard는 아직 후속 경계다. 따라서 이 단계의 테스트는 process 내부 수렴을 증명하지만 process가 죽은 뒤의 복구를 증명하지 않는다.

## Peer transport

MVP transport framing은 `docs/adr/0003-ordered-peer-transport.md`를 따른다. 하나의 ordered stream에서 bounded frame을 순서대로 읽고, transport는 control, input, heartbeat kind와 payload 크기까지만 검증한다. payload decode와 runtime 호출은 인증된 connection을 소유한 adapter 책임이다.

server와 client TLS config는 TLS 1.3만 허용하고 `krasis/1` ALPN을 사용한다. server는 신뢰 root로 검증된 client certificate를 요구하고 client도 명시적인 expected server name과 신뢰 root로 server certificate를 검증한다. system root fallback, insecure skip verify와 익명 연결은 사용하지 않는다.
