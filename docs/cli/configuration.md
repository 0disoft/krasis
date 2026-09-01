# CLI Configuration

- 상태: Active
- 실행 파일: `krasis-lab`, `krasis-launcher`
- 정식 제품 CLI: 미구현

## `krasis-lab`

`krasis-lab`은 Phase 0 실험 CLI다. 현재 global config file, environment-variable override와 implicit
profile을 읽지 않는다. 각 command가 요구하는 flag가 전부이며 사용법, JSON schema와 exit code는
`docs/cli/command-contract.md`와 `docs/cli/output-and-exit-codes.md`가 소유한다.

명령행의 상대 경로 해석은 process working directory를 따른다. report writer는 기존 output을 덮어쓰지
않고 부모 directory, regular file, 크기, JSON schema와 Windows reparse-point 경계를 command별로
검증한다. credential path와 trust-store path는 report나 JSON stdout에 복제하지 않는다.

## `krasis-launcher`

launcher는 executable directory의 `launcher-config.json` 하나만 읽고 쓴다. 다른 search path, registry,
environment override와 command-line override는 없다. 파일이 없으면 UI가 hostname 기반 Phase 0 default를
제안할 수 있지만 저장 전에는 session 설정으로 확정되지 않는다.

schema는 `krasis.launcher.config/v1`이며 최대 64 KiB의 regular JSON file만 허용한다. unknown field,
trailing JSON value와 validation failure를 거부한다. 저장은 같은 directory의 temporary file을 flush한
뒤 replacement로 승격한다.

| field | 계약 |
| --- | --- |
| `schemaVersion` | 정확히 `krasis.launcher.config/v1` |
| `role` | `listen` 또는 `connect` |
| `deviceAlias` | `H-A`, `H-B`, `S-A`, `S-B` |
| `peerHost` | literal IP address |
| `port` | 1부터 65535, 기본 24876 |
| `edge` | listener/A alias는 `RIGHT`, connector/B alias는 `LEFT` |
| `credentialDirectory` | launcher directory 아래 portable relative path |
| `reportsDirectory` | launcher directory 아래 portable relative path, 기본 `reports` |
| `duration` | Go duration, 최소 1초, 제안 default `2m` |

config에는 certificate, private key, trust fingerprint, report body와 raw input을 넣지 않는다.
credential과 report directory는 absolute path, volume-qualified path, `..` escape를 거부한다.
launcher가 만든 session command의 heartbeat `500ms`와 peer timeout `3s`는 Phase 0 launcher 값이지
제품 기본값 약속이 아니다.

## Precedence

현재 precedence는 단순하다.

1. `krasis-lab`은 explicit command flags만 사용한다.
2. `krasis-launcher`는 사용자가 UI에서 저장한 값이 `launcher-config.json` 전체를 교체한다.
3. environment와 registry는 두 실행 파일의 config source가 아니다.
4. code default는 launcher config가 없을 때 초기 form을 채우는 용도이며 저장된 config를 덮지 않는다.

## 아직 결정하지 않은 계약

| 결정 | 책임 | blocker | 결정 trigger |
| --- | --- | --- | --- |
| 정식 `krasis` command tree와 config path | repository maintainers | production agent와 local API 미구현 | #13 완료 후 #14 설계 |
| 정식 CLI JSON versioning과 exit taxonomy | repository maintainers | product command 목록 미확정 | #14 command contract PR |
| environment-variable 지원 여부 | repository maintainers | secret ownership과 installer path 미확정 | #14와 #16 교차 검토 |
| shell completion | repository maintainers | command tree 미확정 | #14 command names 동결 |
