# Network Exposure

- 상태: Active
- 범위: Phase 0 `agent-session listen`
- 기본 port: TCP 24876
- 결정: ADR 0033

## 허용 환경

listener는 사용자가 통제하는 private LAN 또는 personal hotspot에서만 실행한다. launcher의 IP 항목에는
listener 장비가 해당 network interface에서 실제로 가진 literal IP를 넣는다. launcher는 그 IP에만
bind하며 `0.0.0.0`과 `::` wildcard를 사용하지 않는다.

Windows network profile이 Public이거나 network 소유자를 신뢰할 수 없으면 listener를 시작하지 않는다.
호텔, 카페, 공항, 회사 guest Wi-Fi에서 peer가 보이지 않는다고 firewall scope를 넓히거나 public profile을
허용하지 않는다. client isolation이 켜진 network에서는 personal hotspot이나 통제 가능한 private LAN으로
옮긴다.

## 실행 전 확인

PowerShell에서 현재 profile과 선택할 local address를 확인한다.

```powershell
Get-NetConnectionProfile
Get-NetIPAddress -AddressFamily IPv4,IPv6 |
  Where-Object { $_.IPAddress -notin @('127.0.0.1', '::1') }
```

선택한 interface의 profile이 Private인지, listener config의 IP가 그 interface에 실제로 존재하는지,
connector가 같은 network에서 그 IP에 도달할 수 있는지 확인한다. IP가 DHCP로 바뀌면 기존 config를
재사용하지 않고 새 local IP로 저장한다.

## Firewall 계약

Phase 0은 firewall rule을 자동 생성하지 않는다. 수동 rule이 필요하면 다음 범위를 모두 좁힌다.

- program은 현재 probe kit의 exact `krasis-lab.exe`
- protocol은 TCP
- local address는 launcher에서 선택한 listener IP
- local port는 선택한 port 하나
- remote address는 connector의 현재 IP 하나
- profile은 Private만 허용

mTLS와 trust store가 있으므로 firewall을 생략해도 된다는 판단은 금지한다. firewall 통과, TLS identity,
ALPN, certificate role과 inbound-control capability는 서로 다른 gate다.

수동 rule에는 사용자가 식별 가능한 고유 이름을 붙이고 Krasis 실험 외의 기존 rule을 수정하지 않는다.
실험이 끝나면 그 이름의 rule만 제거한다. installer가 rule을 자동 소유하는 계약은 issue #16에서 결정한다.

## Listener admission

인증 전 접속은 주소별 8회 burst와 초당 1회 refill을 적용하고, process 전체의 동시 TLS handshake를
4개로 제한하며 각 handshake를 5초 뒤 취소한다. tracked address는 최대 256개, idle retention은 5분이다.

`ListenSource.PreAuthMetrics`는 authenticated acceptance, pending round 없음, 주소 제한, global capacity,
handshake timeout, authentication failure와 cancellation의 count만 반환한다. IP, port, certificate,
session과 credential identifier는 metric label이나 durable report에 넣지 않는다.

## 차단·오작동 복구

연결이 실패하면 firewall을 넓히기 전에 listener를 중단하고 두 장비가 같은 private network인지,
선택한 local IP가 아직 유효한지, connector remote IP가 rule과 일치하는지 확인한다. listener 존재 여부는
다음처럼 확인한다.

```powershell
Get-NetTCPConnection -State Listen -LocalPort 24876
```

잘못 만든 Krasis 전용 rule은 Windows Defender Firewall UI에서 exact 이름과 program path를 확인한 뒤
삭제한다. 다른 application rule이나 전체 firewall profile을 끄지 않는다. listener process를 종료하고
listening socket이 사라진 것을 확인한 뒤 config와 rule을 고쳐 다시 실행한다.

network 문제 때문에 input control이 불확실해지면 remote 입력을 계속 시도하지 않는다. local emergency
release 또는 process 종료로 `ReleaseAll`과 `LOCAL` 수렴을 먼저 수행한다.
