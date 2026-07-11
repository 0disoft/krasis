# ADR 0005: session 의미 순서와 heartbeat write를 한 coordinator가 소유한다

- 상태: Accepted
- 책임자: cherr

## 맥락

ADR 0003은 한 peer connection의 frame byte write를 직렬화한다. 하지만 현재 runtime 전이와
frame write는 같은 의미 단위로 직렬화되지 않는다. 별도 goroutine의 heartbeat sender가
active snapshot을 읽은 뒤 destination return이 runtime을 `LOCAL`로 만들고 `LeaseRevoke`를
먼저 쓸 수 있다. 이때 늦은 heartbeat가 revoke 뒤에 기록되면 peer는 이미 끝난 session의
heartbeat를 받고 connection fault로 처리한다.

writer mutex만으로는 frame bytes가 섞이지 않는다는 것만 보장한다. `LeaseRequest`, grant,
reject, revoke, input과 heartbeat 사이에서 어떤 runtime 결정이 어떤 frame보다 먼저였는지는
보장하지 않는다. heartbeat cadence와 timeout도 아직 Phase 0 측정 전이라 제품 기본값을
정할 근거가 없다.

## 결정

- authenticated peer connection마다 `SessionWriteCoordinator` 하나가 control transition과
  outbound semantic frame 순서를 소유한다.
- heartbeat scheduler, boundary controller, lease responder, return controller와 input sender는
  connection writer를 직접 호출하지 않는다. 모두 같은 coordinator operation을 사용한다.
- peer에서 받은 grant, reject와 revoke처럼 outbound 전송 상태를 바꾸는 control dispatch도
  coordinator를 거친다. read loop 자체를 잠그지는 않지만 해당 command의 runtime 적용은
  concurrent heartbeat 또는 local revoke operation과 같은 의미 순서에 놓인다.
- 각 operation은 coordinator lock 안에서 현재 session, lease, epoch와 runtime state를 다시
  확인하고 필요한 transition 또는 input sequence를 예약한 뒤 frame을 encode하고 write한다.
  runtime 내부 mutex를 network I/O 동안 직접 잡아 두지는 않는다.
- write 성공 여부가 불확실하면 sequence나 transition을 되돌려 재사용하지 않는다. 기존
  session을 fail-safe cleanup하고 새 connection과 새 session으로만 재시작한다.
- heartbeat는 `INBOUND_ACTIVE` 또는 `OUTBOUND_ACTIVE`인 정확한 현재 session에만 보낸다.
  `LOCAL`, `PENDING`, `REVOKING`, `FAULT`에서는 보내지 않는다. revoke가 먼저 coordinator를
  획득하면 그 뒤의 heartbeat는 frame을 쓰지 않고 폐기한다. heartbeat가 먼저 획득하면 해당
  frame write가 끝난 뒤 revoke가 기록된다.
- heartbeat interval과 peer timeout은 caller가 명시적으로 주입하고 timeout은 interval보다
  길어야 한다. 제품 기본값은 실제 두 장비의 congestion과 절전 실험 전까지 `UNDECIDED`다.
  Phase 0 report에는 실행에 사용한 두 값을 기록하고 pair 및 순차 집계에서 일치 여부를
  검사한다.
- Phase 0 계측은 성공한 heartbeat 송수신 횟수와 같은 session 내부 수신 gap만 aggregate한다.
  session ID는 재연결 gap reset에만 사용하고 report에 저장하지 않는다. observer는 성공한
  semantic operation 밖에서 짧은 in-memory 집계만 수행하며 network 또는 disk I/O를 하지 않는다.
- timeout 판정은 peer timestamp가 아닌 local monotonic observation을 사용하며, coordinator를
  획득한 뒤 현재 시각을 읽어 대기 시간을 deadline에서 빼먹지 않는다. timeout이나
  heartbeat write failure는 bounded cleanup context에서 `ReleaseAll`과 `LOCAL` 수렴을
  시도한다. 이 cleanup context는 session 취소를 상속하지 않아 동시 cancellation이 held input
  release를 선점하지 못한다.
- `HEARTBEAT` payload v1의 session ID 계약은 바꾸지 않는다. epoch 추가 필요성은 실제 stale
  session 충돌 증거가 생길 때 별도 protocol ADR로 판단한다.

## 검증 계약

- heartbeat write가 완료된 뒤 revoke가 기록되는 순서
- revoke가 먼저 확정되면 대기 중 heartbeat가 기록되지 않는 순서
- grant 전과 cleanup 뒤에는 heartbeat가 기록되지 않음
- heartbeat 중단 시 destination의 held input이 `ReleaseAll`되고 양쪽이 `LOCAL`로 수렴함
- heartbeat write가 block되거나 실패해도 coordinator와 runtime이 무기한 잠기지 않음
- coordinator 대기 중 deadline이 지나면 과거 ticker 시각으로 heartbeat를 보내지 않음
- deadline 판정과 session cancellation이 겹쳐도 held input cleanup이 완료됨
- 서로 다른 interval 또는 timeout의 Phase 0 evidence가 통과하지 않음
- heartbeat 송수신과 같은-session gap evidence가 없는 Phase 0 report는 통과하지 않음

## 결과

heartbeat timer를 바로 붙이는 것보다 coordinator 도입이 먼저다. 이 순서를 뒤집으면
byte-level ordered stream 위에서 application 의미 순서가 다시 깨진다. coordinator는 input
hot path에 lock 하나를 추가하므로 실제 polling-rate 측정에서 contention을 확인해야 한다.

`SessionWriteCoordinator`는 boundary request, grant/reject/revoke dispatch, destination grant,
return revoke, input, heartbeat write와 failure cleanup에 연결됐다. coordinator lock을 기다리는
중에도 context 취소가 가능하고, read/dispatch failure는 connection을 먼저 닫아 막힌 write를
깨운 뒤 bounded cleanup을 수행한다. source suppression은 cleanup 대기 전과 coordinator를
획득한 뒤에 모두 해제해 concurrent activation 뒤 다시 켜진 상태를 남기지 않는다.
inbound input과 heartbeat dispatch도 같은 coordinator operation에서 실행하며 malformed control은
직접 runtime을 변경하지 않고 connection close 뒤 coordinated cleanup으로 넘긴다.

이 ADR은 heartbeat 숫자를 확정하거나 별도 heartbeat connection을 허용하지 않는다. 단일
ordered stream에서 head-of-line blocking이 실제로 확인될 때만 ADR 0003을 대체하는 transport
결정을 추가한다. caller가 명시적으로 `heartbeat interval < peer timeout` policy를 주입할 때만
동작하는 주기 sender와 deadline worker는 구현됐다. policy가 없으면 worker를 시작하지 않으며
제품 기본값은 여전히 `UNDECIDED`다. `agent-session`은 두 값을 필수로 받아 v4 report에
millisecond 단위로 기록하며 pair checker와 순차 Phase 0 checker가 각각 같은 장소와 네 장비
전체의 값 일치를 검증한다. 장소별 수집 날짜가 달라도 이 계약은 동일하다.
