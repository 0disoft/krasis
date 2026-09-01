# Dependency and Change Policy

- 상태: Active
- 현재 Go runtime dependency: 없음
- 자동 제안: Renovate dependency dashboard

## 기본 정책

새 dependency는 직접 구현 비용보다 유지·보안·배포 비용이 확실히 낮을 때만 추가한다. Krasis의 input,
trust와 recovery 경계는 작은 코드와 표준 library를 우선하며 convenience wrapper 하나를 위해 runtime
dependency를 들이지 않는다. dependency를 추가하면 목적, 대안, license, maintainer 상태, known
vulnerability, transitive graph, binary/runtime 영향과 제거 경로를 PR에 기록한다.

## Version Policy

같은 major 안에서는 안정 patch와 minor를 따라가고, 새 major는 별도 PR에서 CI 호환성을 확인한 뒤
의도적으로 승격한다. Go module은 Go MVS가 요구하는 exact version을 `go.mod`와 `go.sum`에 기록하지만
선택 정책은 현재 major의 최신 compatible stable이다. 이유 없이 오래된 exact version에 고정하지 않는다.

GitHub Actions는 supply-chain 변경을 막기 위해 full immutable commit SHA로 pin하고 comment에 release
version을 남긴다. tag만 참조하지 않는다. action major update는 setup, permission, artifact와 cache
동작을 전체 Windows workflow로 검증한다.

Go toolchain floor는 `go.mod` 하나가 소유한다. workflow와 문서는 별도 버전 문자열을 source로 만들지
않고 `go-version-file`을 사용한다.

## Change Isolation

dependency update는 기능 변경, 대규모 refactor와 섞지 않는다. patch/minor는 한 action 또는 한 module
단위, major는 단독 PR이 기본이다. generated lockfile과 manifest diff는 실제 tool output만 commit하고
손으로 내용을 꾸미지 않는다.

security fix가 급하면 affected dependency와 최소 call site만 올리고 unrelated cleanup을 미룬다.
rollback은 이전 immutable pin 또는 module version으로 한 commit에서 가능해야 한다.

## 검증

- Go module 변경: `go mod tidy` 결과 검토, `go test ./...`, vet, 두 Windows executable build
- GitHub Action 변경: 변경된 action으로 complete Windows job 실행
- input hot path dependency: allocation, latency와 failure behavior를 기존 budget과 비교
- crypto, parser, storage dependency: malformed input, downgrade, migration와 recovery test
- installer 또는 UI dependency: artifact size, license, signing와 uninstall 영향 기록

현재 module dependency가 없으므로 새 `require`는 architecture review 대상이다. standard library 또는
기존 package로 작은 범위에서 해결할 수 있으면 dependency를 거절한다.

## 공급망과 개인정보

install script를 pipe로 실행하거나 unverified binary를 CI에서 내려받지 않는다. action permission은
최소로 유지하고 secret을 fork PR과 artifact에 노출하지 않는다. dependency telemetry, crash upload와
network call은 기본 금지이며 제품 명세와 privacy review 없이 켜지 않는다.

## 제거 기준

더 이상 호출되지 않거나 standard library가 대체한 dependency는 즉시 제거한다. 제거 PR은 module,
generated file, license notice, config와 CI cache를 함께 정리한다. abandoned dependency가 trust,
transport 또는 input boundary에 남아 있으면 release blocker다.
