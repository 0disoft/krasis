# ADR 0033: Pre-Auth Listener Admission and Network Exposure

- 상태: Accepted
- 결정일: 2026-09-01
- 범위: TCP accept부터 mTLS·ALPN 인증 완료 전까지의 listener admission과 Phase 0 LAN 노출
- 관련: ADR 0003, ADR 0032, issue #11

## 배경

mTLS, ALPN, certificate role과 trust-store capability는 인증된 session을 보호하지만 TCP accept와 TLS
handshake 자체가 소비하는 socket, goroutine과 CPU는 인증 전에 발생한다. 기존 listener는 동시에 네 개의
handshake만 허용하고 각 handshake를 5초로 제한했으나, 한 주소의 반복 접속과 여러 주소를 흉내 낸
접속을 분리해 관찰하지 못했다. Phase 0 launcher도 사용자가 고른 주소 대신 wildcard에 bind해 선택하지
않은 interface까지 listener를 노출했다.

per-address 제한만 두면 분산 주소가 전체 listener를 고갈시키고 NAT 뒤의 정상 reconnect burst를 쉽게
막는다. 전역 token bucket만 두면 한 주소가 정상 peer의 기회를 모두 소비한다. firewall만 믿으면 lab
runtime의 bounded-resource 보장이 사라지고 잘못된 profile·rule 상태를 application이 알 수 없다.

## 결정

전역 동시성, 주소별 burst, handshake deadline과 외부 firewall 정책을 함께 쓰는 hybrid admission을
채택한다.

- listener는 사용자가 선택한 literal local IP와 port에만 bind한다. IPv4·IPv6 wildcard bind는 launcher
  plan에서 만들지 않는다.
- 인증 전 handshake는 process 전체에서 최대 4개만 동시에 실행하고 각 handshake는 5초 안에 끝나야 한다.
- remote address의 port를 버리고 IP 단위 token bucket을 적용한다. 기본 burst는 8회, refill은 1초에
  1회다. 같은 NAT 주소의 정상 reconnect가 짧게 몰려도 즉시 차단하지 않는다.
- address bucket은 최대 256개만 유지하고 5분 동안 관찰되지 않은 entry를 제거한다. 가득 찼으면 가장
  오래 관찰되지 않은 entry 하나를 버린다. DHCP나 분산 주소가 application memory를 무한히 늘릴 수 없다.
- pending `Next`가 없는 접속, 주소별 제한, 전역 capacity, handshake timeout, authentication failure,
  cancellation과 authenticated acceptance를 원자적 aggregate counter로만 노출한다.
- 주소는 in-memory admission key로만 잠시 유지한다. log, report, metric label과 durable file에 IP나
  port를 기록하지 않는다.
- rate limit 통과는 인증이 아니다. TLS 1.3 mTLS, `krasis/1` ALPN, certificate validity·role와 live
  trust-store authorization은 서로 독립된 gate로 유지한다.
- Phase 0 listener는 사용자가 통제하는 private LAN 또는 personal hotspot에서만 실행한다. public network
  profile에서는 선택한 executable, local address·port, Private profile과 exact peer address로 제한한
  명시적 firewall policy가 없으면 listener를 실행하지 않는 것이 운영 계약이다.
- Phase 0은 firewall rule을 자동 생성하거나 삭제하지 않는다. installer가 소유한 reversible firewall
  automation은 issue #16에서 별도로 결정한다.

## 결과

한 주소의 partial handshake는 burst 뒤 즉시 닫히고, 여러 주소의 동시 공격은 네 개의 global slot과
5초 deadline 안에 갇힌다. NAT 뒤 정상 reconnect는 8회 burst를 공유한다. DHCP로 주소가 바뀌면 새
bucket을 받지만 tracked state는 256개로 제한된다.

per-address limiter는 대규모 분산 source를 식별하지 못하고 같은 NAT의 여러 정상 peer를 구분하지 못한다.
그 잔여 위험은 global cap과 firewall scope가 담당한다. public Wi-Fi client isolation 때문에 정상 peer가
서로 보이지 않는 경우 firewall을 넓히거나 wildcard bind로 우회하지 않고 private LAN·hotspot으로 옮긴다.

## 검증

- `TestAddressRateLimiterAllowsBurstThenRefills`
- `TestAddressRateLimiterSharesBudgetAcrossPorts`
- `TestAddressRateLimiterBoundsTrackedAddressesAndExpiresIdleEntries`
- `TestListenSourceRecordsCapacityAndHandshakeTimeout`
- `TestListenSourceRateLimitsBeforeStartingAnotherHandshake`
- `TestListenSourceCountsAuthenticatedPeerWithoutPersistingAddress`
- `TestListenSourceDoesNotLetStalledHandshakeBlockAuthenticatedPeer`
- `TestBuildSessionPlanBindsListenerToConfiguredInterface`
