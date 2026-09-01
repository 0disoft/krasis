# Threat Model

- 상태: Active
- 범위: Phase 0 lab runtime과 목표 Windows peer-control product
- 관련 기준: security baseline, ADR 0003부터 ADR 0012, risk register

## 보호 자산

Krasis가 보호해야 하는 핵심 자산은 로컬 사용자의 입력 통제권, trusted peer authorization,
device private key, active session·lease identity, ordered input stream, pressed-input recovery state와
진단 자료의 개인정보다. 가용성 실패가 confidentiality 실패만큼 중요하다. 사용자가 키보드나 마우스를
되찾지 못하면 제품은 실패한 것이다.

## Trust Boundaries

- physical input에서 Windows hook/Raw Input adapter로 들어오는 OS boundary
- source agent와 destination agent 사이 TLS 1.3 mTLS framed connection
- peer certificate에서 local trust store capability로 이어지는 authorization boundary
- launcher/UI와 resident agent 사이의 future local-control boundary
- agent와 crash guard 사이 inherited anonymous pipe
- config, trust store, credential와 report를 쓰는 local filesystem boundary
- source tree, pinned GitHub Actions와 distributed probe-kit 사이 supply-chain boundary

## 공격자와 실패 원인

- 같은 LAN에서 handshake 자원을 소모하거나 malformed frame을 보내는 미인증 peer
- 유효 certificate를 가졌지만 revoke됐거나 inbound capability가 없는 peer
- 이전 session, lease, epoch와 sequence를 replay하는 stale peer
- 같은 Windows 사용자 권한으로 file, executable, pipe 또는 report path를 바꾸는 local process
- compromised dependency, mutable CI action tag 또는 변조된 probe kit
- network 단절, sleep, lock, process crash, partial injection과 cleanup failure
- 사용자가 raw report, credential 또는 local path를 실수로 공유하는 운영 오류

## 필수 방어

모든 application session은 TLS 1.3 mTLS와 `krasis/1` ALPN 뒤에만 시작한다. certificate identity는
local trust store record와 capability를 통과해야 하며 revoke record를 같은 fingerprint import로
부활시키지 않는다.

control과 input은 exact session, lease, epoch와 monotonic sequence를 검사한다. duplicate, gap,
out-of-order, stale identity, invalid action과 uncertain injection은 추측 실행하지 않고 current session을
cleanup한다. destination prepare가 끝나기 전 source suppression을 시작하지 않는다.

down 주입 전 `PressedState`와 crash guard에 복구 책임을 기록하고, successful up 뒤에만 제거한다.
revoke, fault, disconnect와 lifecycle termination은 모든 token의 역순 release를 시도한다. 남은 token이
있으면 `LOCAL`로 가장하지 않는다.

local JSON은 bounded strict schema로 읽고 unknown field와 trailing value를 거부한다. trust/config/report
write는 같은 directory의 temporary file을 flush한 뒤 atomic replacement 또는 no-replace promotion을
사용한다. credential과 trust directory는 protected DACL을 확인한다. report는 aggregate만 저장하고 raw
input과 local identity를 금지한다.

CI action은 immutable SHA로 pin한다. probe kit는 manifest와 SHA-256을 검증하며 source revision과 연결한다.

## Phase 0 Crash Guard Boundary

guard는 agent와 같은 사용자·권한으로 실행하고 별도 network listener나 elevated helper가 아니다.
IPC는 parent가 만든 anonymous pipe handle만 상속하며 versioned fixed-size command와 bounded token
수만 허용한다. payload는 normalized key/button kind와 code만 포함하고 session, peer, 주소, credential,
pointer path, window 정보와 입력 문자열을 금지한다.

pipe 단절과 protocol 오류는 입력 계속이 아니라 남은 token release와 guard 종료로 수렴한다.
agent는 guard 선종료를 session failure로 취급한다. report는 token code를 버리고 reason과 aggregate
count만 기록한다.

같은 사용자 권한의 악성 process, executable replacement, guard와 agent 동시 종료, 전원 차단과 실제
physical key 충돌은 guard만으로 제거하지 못한다. installer signing, process layout와 physical evidence가
남은 방어선이다.

## Phase 0 Network Exposure Boundary

listener는 pre-auth handshake concurrency를 4개로 제한하고 각 handshake를 5초로 제한한다.
이는 인증되지 않은 client의 application 진입을 막지만 TCP accept와 certificate handshake 자원 소모를
없애지 않는다. IP별 rate limit과 자동 firewall rule은 현재 없다.

따라서 Phase 0 listener는 신뢰하는 private LAN 또는 personal hotspot에서만 연다. public LAN에서는
connector 주소만 허용하는 firewall rule을 사용자가 확인하지 못하면 실행하지 않는다.

## Residual Risk and Decision Triggers

| 위험 | 현재 처리 | 책임 | 다음 결정 trigger |
| --- | --- | --- | --- |
| same-user executable/file replacement | bounded path와 DACL 검증, 완전 차단 아님 | repository maintainers | #16 signing/install layout |
| power loss와 agent/guard 동시 종료 | in-memory recovery만 존재 | repository maintainers | #4부터 #7 physical fault evidence |
| public LAN resource exhaustion | concurrency/deadline 제한, per-IP limiter 없음 | repository maintainers | installer firewall UX와 reconnect policy 확정 |
| production local UI API abuse | API 미구현, UI도 미구현 | repository maintainers | #13 완료 뒤 #15 설계 |
| certificate rotation과 expiry UX | Phase 0 credential 수동 관리 | repository maintainers | #10 production identity workflow |
| raw diagnostic 공유 실수 | aggregate schema와 docs 금지 | repository maintainers | #18 beta support-bundle UX |

## Review Blockers

authentication·authorization을 하나로 취급하거나, stale input을 재생하거나, cleanup failure를 숨기거나,
raw input·credential·local identifier를 log/report에 넣거나, UI를 input safety owner로 만드는 변경은
merge하지 않는다.
