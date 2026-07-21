# Threat Model

- Status: Draft

## Contract

Operability standard connects code changes to logs, metrics, traces, rollback, runbooks, health checks, incident response, and failure evidence.

## Required Evidence

- Source of truth: UNDECIDED
- Owner: UNASSIGNED
- Merge-blocking validation: VALIDATION.md
- Related checklist: CHECKLIST.md

## Phase 0 Crash Guard Boundary

- 관련 결정은 `docs/adr/0004-phase-0-crash-guard.md`다.
- guard는 agent와 같은 사용자·권한으로 실행하고 별도 network listener나 elevated helper가 아니다.
- IPC는 parent가 만든 익명 pipe handle만 상속하며 12-byte versioned command, 8-byte ACK와
  최대 256개 token만 허용한다.
- payload는 normalized key/button kind와 code만 포함한다. session, peer, 주소, 자격 증명,
  pointer 경로, window 정보와 입력 문자열은 금지한다.
- 종료 evidence는 token code를 버리고 reason과 aggregate count만 기록한다. output은 사용자가
  지정한 기존에 없는 JSON이며 agent report는 basename만 참조해 directory traversal을 막는다.
  report writer는 부모 path component의 symlink·Windows reparse point를 거부하고 승격 직전 다시
  검사한다. 같은 사용자 권한의 path 교체 경쟁까지 막는 handle-relative writer는 아직 없다.
- protocol 오류나 pipe 단절은 fail-open 입력 계속이 아니라 남은 token 해제와 guard
  종료로 수렴한다. agent는 guard 선종료를 session failure로 취급한다.
- 같은 사용자 권한의 악성 process, executable 교체, guard와 agent 동시 종료, 전원 차단과
  실제 물리 key 충돌은 Phase 0 실장비 및 설치·서명 경계 전까지 잔여 위험이다.

## Phase 0 Network Exposure Boundary

- TLS 1.3 mTLS와 ALPN은 인증되지 않은 client가 application session에 들어오는 것을 막지만 TCP accept와
  certificate handshake 자원 소모 자체를 없애지는 않는다.
- listener는 pre-auth handshake를 동시에 4개까지만 허용하고 각 연결을 5초로 제한한다. IP별 rate limit,
  자동 Windows 방화벽 규칙과 공용 LAN 공격 완화는 아직 구현하지 않는다.
- 따라서 신뢰하는 사설 LAN 또는 개인 hotspot이 기본 운영 경계다. 공용 LAN에서는 connector 주소만
  허용하는 방화벽 규칙을 사용자가 확인할 수 없으면 listener를 열지 않는다.
- 이 경계는 인증 우회 방어가 아니라 가용성 방어의 잔여 위험이다. IP별 limiter는 NAT, DHCP 주소 변경과
  정상 reconnect 정책을 함께 결정한 뒤 별도 설계와 부하 검증으로 도입한다.

## Review Blockers

- A change bypasses the source of truth.
- A change weakens validation or hides skipped checks.
- A change lacks failure, recovery, security, performance, or test evidence where relevant.
