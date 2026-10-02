# LAN 주소 변경 복구

g122부터 연결 시도에 실패한 connector는 private IPv4 LAN에서 상대의 새 접속 후보를
찾는다. 기존 주소가 정상인 연결에는 검색이나 추가 연결을 넣지 않는다.

- UDP `45874`는 16바이트 임의 nonce에 입력 TCP port만 응답한다. fingerprint, alias,
  기기명, 인증서, PIN은 광고하지 않는다. 응답은 신뢰하지 않는 routing hint다.
- 검색은 1.5초·최대 8개 후보이며 실패 시 최대 15초에 한 번 수행한다. 전체 획득 시도는
  12초, 기존 주소는 5초, 후보별 TLS 연결은 1초로 제한한다.
- 저장된 TCP port와 private/loopback 주소만 후보로 사용한다. 기존 mTLS 인증서 pin,
  TLS 역할, 만료·현재 trust 검사를 통과해야 한다. 다른 identity에 입력을 보내지 않는다.
- 검증된 새 주소만 동일 fingerprint와 이전 주소를 비교해 저장한다. 경계·재진입 거리와
  신뢰/입력 허용 설정을 보존하며 동시 주소 수정이나 trust 변경을 덮어쓰지 않는다.
- listener는 처음 선택한 NIC의 private IPv4 변경만 따라간다. 주소가 없어지면 대기 중인
  accept를 깨운 뒤 재획득에서 listener를 재생성한다. 원래 주소가 유지되면 재생성하지
  않는다. 대체 주소가 여러 개이거나 어댑터가 사라지면 자동 선택하지 않는다.
- 주소 저장 후 관리 채널도 기존 profile reconciliation을 통해 새 주소를 사용한다.
  입력 lease나 이전 세션의 눌린 입력을 재사용하지 않는다.

검색은 같은 LAN의 private IPv4와 양쪽 앱의 연결 의도 활성 상태를 대상으로 한다.
UDP가 방화벽·AP 격리로 차단되거나, IPv6·서로 다른 subnet·어댑터 교체·listener 재시작
전 주소 변경이면 수동 주소 설정이 필요할 수 있다. 보안 정책·방화벽 규칙은 자동 변경하지
않는다. 기존 상대 버전이 광고를 지원하지 않아도 저장 주소 연결은 유지된다.

진단에는 `address_discovery_started`, `address_discovery_unavailable`,
`address_candidate_rejected`, `address_discovery_confirmed`, `address_discovery_exhausted`,
`address_save_failed`, `address_advertisement_unavailable`, `address_listener_rebound` 코드만
남긴다. 주소 원문·어댑터 식별자·인증서는 기록하지 않는다.

로컬 검증은 UDP loopback, nonce/public 주소 거부, 같은 인증 identity의 새 endpoint와
다른 identity 거부, profile 비교 저장, listener accept 중단과 같은 IP 복구를 포함한다.
실제 HP15↔GRAM13 DHCP 변경·방화벽 환경 검증은 별도로 필요하다. GitHub #70은
그 실기기 완료 조건을 확인하기 전까지 열어 둔다.
