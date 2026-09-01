# Shell completion

- Status: Deferred
- Owner: installer issue #16
- Command source: `docs/cli/command-contract.md`

현재 binary는 completion script를 생성하거나 설치하지 않는다. 제품 명령은 확정됐지만 unsigned 개발
binary의 설치 위치와 PowerShell profile mutation을 먼저 고정하면 제거·rollback 계약이 생기기 때문이다.
서명된 installer가 설치/제거 ownership을 확정할 때 PowerShell completion을 command contract에서 생성하고
일반 사용자 profile에 명시적 opt-in으로 설치한다.

completion은 alias, pairing code, fingerprint, token, endpoint나 local path를 history 또는 suggestion
cache에서 수집하지 않는다. command와 option 이름만 정적으로 완성한다.
