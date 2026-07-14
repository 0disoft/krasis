# ADR 0003: 단일 ordered peer transport와 bounded frame

- 상태: Accepted
- 책임자: cherr

## 맥락

Krasis는 control lease, heartbeat와 입력 event의 의미 순서를 보존해야 한다. control과 input을 독립 연결로 먼저 나누면 서로 다른 연결의 장애와 재연결 순서를 다시 합치는 protocol이 추가된다. 반대로 크기 제한 없는 message나 payload까지 transport가 해석하면 입력 hot path와 보안 경계가 뒤섞인다.

## 결정

- MVP peer session은 하나의 ordered byte stream을 사용한다.
- 실제 네트워크 연결은 TLS 1.3 mutual authentication과 `krasis/1` ALPN 협상을 통과한 뒤에만 frame을 전달한다. transport는 검증된 leaf certificate의 SHA-256 fingerprint와 EKU 기반 `LISTENER`/`CONNECTOR` 역할을 `AuthenticatedPeerContext`로 고정해 session과 authorization policy에 전달한다. 인증서 발급, 사용자 pairing 확인, fingerprint allowlist와 revocation 방식은 별도 ADR 전까지 `UNDECIDED`다.
- wire frame은 magic, protocol version, message kind, reserved flags와 payload length를 가진 고정 header 뒤에 opaque payload를 둔다.
- payload는 최대 65,536 bytes다. 이 값은 작은 control/input message에 비해 충분한 여유를 두면서 peer가 선언하는 메모리 사용량을 제한하기 위한 초기 방어선이며, 실제 payload 측정 뒤 축소할 수 있다.
- 정의되지 않은 version, message kind, flags, oversized payload와 truncated frame은 연결 오류다. 부분 frame을 추측하거나 재생하지 않는다.
- `CONTROL`, `INPUT`, `HEARTBEAT` kind만 transport가 구분한다. payload serialization과 Windows input code mapping은 이 ADR에서 결정하지 않는다.
- 한 session 안의 writer는 직렬화한다. 여러 goroutine이 같은 writer에 직접 frame을 섞어 쓰지 않는다.

`INPUT` payload version 1은 session ID, epoch, sequence와 정규화 action을 big-endian binary로 담는다. session ID와 epoch는 각각 최대 255 bytes이며 payload는 정확한 길이만 허용한다. action kind와 field 조합, reserved field, zero sequence, trailing bytes가 유효하지 않으면 runtime에 전달하지 않는다.

`HEARTBEAT` payload version 1은 최대 255 bytes의 session ID만 담는다. peer wall-clock이나 timestamp는 전송하지 않고 수신측의 local observation time만 liveness에 사용한다.

`CONTROL` payload version 2는 command kind, session ID, lease ID, epoch와 source boundary edge를 담는다. `LeaseRequest`만 필수 `LEFT` 또는 `RIGHT` edge를 포함하고 grant, reject와 revoke에는 edge가 없어야 한다. destination은 source의 `RIGHT` request를 local virtual desktop의 왼쪽 안쪽으로, source의 `LEFT` request를 오른쪽 안쪽으로 배치한 뒤에만 inbound lease와 grant를 확정한다. 정의되지 않은 edge code, command별 금지 field, 길이 불일치와 reserved field는 payload 오류다. version 1 control payload와의 호환은 출시 전 계약이므로 유지하지 않는다.

## 결과

control과 input 사이의 순서를 하나의 stream에서 설명할 수 있고 frame decode 전에 메모리 상한을 적용할 수 있다. 대신 대량 input이 heartbeat를 늦추는 head-of-line blocking 가능성이 남는다. 실제 두 기기 측정에서 이 문제가 확인될 때에만 독립 channel과 cross-channel epoch 규칙을 새 ADR로 도입한다.

`net.Pipe` loopback 검증은 framing과 순서를 증명하지만 TLS, Wi-Fi 장애, 방화벽, 지연 또는 packet loss를 증명하지 않는다.

TLS loopback 검증은 양쪽 인증서 검증, TLS version과 ALPN을 증명한다. test 인증서는 메모리에서만 생성하며 제품 pairing과 certificate storage를 대신하지 않는다.

grant 전 destination cursor 이동의 prepare·commit·rollback 순서와 network write 실패 보상은
ADR 0009가 이 결정의 후속 효과 계약으로 보완한다.
