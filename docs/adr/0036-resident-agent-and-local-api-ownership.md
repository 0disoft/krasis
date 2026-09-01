# ADR 0036: Resident agent와 인증된 local API 소유권

- 상태: Accepted
- 결정일: 2026-09-01
- 범위: 제품 agent process, single instance, local API, bounded shutdown
- 관련 ADR: `0001-initial-architecture-boundaries.md`, `0007-windows-agent-lifecycle-owner.md`, `0034-exact-active-lease-management-revocation.md`

## 배경

`krasis-lab`은 제한 시간과 명시적 live-input 승인 아래 실행되는 실험 CLI이고 launcher는 그 child
process만 소유한다. 제품 UI나 CLI가 hook, peer session, trust mutation 또는 `ReleaseAll`을 직접
소유하면 process 종료와 reconnect에서 제어권이 갈라진다.

## 결정

- `krasis-agent.exe`가 현재 interactive user session에서 hook·transport·trust·session runner와
  lifecycle cleanup의 유일한 owner다. UI와 CLI는 local API client일 뿐이다.
- `Local\Krasis-Agent-v1` Windows kernel mutex를 initial owner로 획득한다. 기존 owner가 있으면 두 번째
  process는 종료한다. process가 handle을 닫지 못하고 죽어도 kernel object lifetime이 끝나므로 다음
  실행이 stale file 판단 없이 새 owner를 획득한다.
- 시작 순서는 single-instance acquisition, `%LOCALAPPDATA%\Krasis` identity bootstrap, Windows
  lifecycle monitor, loopback listener, local credential publication이다.
- local API는 `127.0.0.1`의 임의 port에만 bind한다. 매 process 시작마다 256-bit bearer token을 만들고
  address와 token을 `%LOCALAPPDATA%\Krasis\control`의 서로 다른 protected file에 기록한다.
  directory와 file은 현재 사용자와 `SYSTEM`만 허용한다.
- request는 bearer token을 constant-time 비교한 뒤에만 처리한다. body는 64KiB 이하 strict JSON이며
  unknown field, trailing JSON, 잘못된 operation별 field 조합을 application 호출 전에 거부한다.
- API operation은 status, connect, disconnect, pair begin/confirm/cancel, aggregate diagnostics와 safe stop이다.
  response는 stable status/code와 redacted projection만 반환하고 private key, certificate body, session·lease,
  주소, path와 내부 error string을 포함하지 않는다.
- connect로 시작한 shared session runner는 HTTP request context가 아니라 agent root context의 child다.
  UI/CLI 종료는 session을 중단하지 않는다. disconnect, lifecycle event와 safe stop만 session cancel을
  수행하며 5초 안에 종료되지 않으면 `shutdown_timeout`을 남긴다.
- runner port는 기존 `internal/agent` supervisor/session core를 호출하는 adapter 자리다. resident agent는
  별도 input/session 구현을 만들지 않는다. #14의 product configuration adapter가 없으면 connect와 pair는
  `not_configured`를 반환한다.
- lock, suspend, resume, logoff, shutdown과 display change는 기존 Windows lifecycle owner가 agent root를
  취소한다. local API를 먼저 닫고 shared session을 cancel/wait한 뒤 monitor와 instance handle을 닫는다.

## 결과

UI나 CLI가 crash해도 입력 cleanup owner가 살아 있고, stale owner recovery는 PID·timestamp file 추측에
의존하지 않는다. loopback socket 자체는 same-user authorization을 증명하지 않으므로 protected bearer
token이 필수다. 같은 사용자 권한을 이미 획득한 process는 product trust boundary 안에 있으며, 다른
Windows 사용자나 원격 peer는 token file을 읽을 수 없어야 한다.

## 검증

- 같은 mutex의 두 번째 owner 거부와 unclosed subprocess 종료 뒤 재획득
- endpoint/token DACL, loopback-only address, strict decode와 cleanup
- unauthenticated request 거부와 operation dispatch
- request cancellation 뒤 shared session 생존, disconnect와 safe stop cancellation
- Windows lifecycle event 뒤 API, session, monitor와 instance의 bounded cleanup
- Windows CI에서 `krasis-agent.exe` build
