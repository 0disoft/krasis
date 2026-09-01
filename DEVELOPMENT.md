# Development

- 상태: Active
- 주 개발 환경: Windows 11, PowerShell 7, Go `go.mod` 버전
- 전체 검증 환경: GitHub Actions `windows-latest`

## 저장소 구성

`cmd/krasis-lab`은 Phase 0 증거 수집 CLI, `cmd/krasis-launcher`는 probe kit용 Win32 launcher다.
`internal/control`은 순수 상태와 순서 모델, `internal/application`은 serialized runtime,
`internal/peer`와 `internal/transport`는 인증된 session orchestration과 frame,
`internal/wininput`은 Windows hook와 `SendInput`, `internal/agent`와 guard package는 lifecycle과
복구 경계를 소유한다. 제품 계약은 `docs/`, 실행 결정은 accepted ADR이 소유한다.

## 준비

Go 버전은 `go.mod`에서 읽는다. 별도 runtime dependency와 package manager는 현재 없다.
Windows SDK를 직접 링크하지 않고 Go의 syscall boundary에서 Win32 API를 동적으로 사용한다.
빌드와 테스트 output은 `build/`, `dist/`, `.cache/` 아래에만 두고 commit하지 않는다.

```powershell
go version
go env GOOS GOARCH
go test ./...
```

Windows가 아닌 환경에서는 순수 package를 targeted test로 확인할 수 있지만, 전체 저장소의 authoritative
결과로 쓰지 않는다. hook, clipboard, lifecycle message, launcher와 probe-kit build는 Windows CI가 기준이다.

## 표준 검증

```powershell
$goFiles = @(Get-ChildItem -Recurse -File -Filter '*.go' | Select-Object -ExpandProperty FullName)
$unformatted = @($goFiles | ForEach-Object { gofmt -l $_ })
if ($unformatted.Count -gt 0) { $unformatted; exit 1 }

go test ./...

$packages = @(go list ./... | Where-Object { $_ -ne 'github.com/0disoft/krasis/internal/wininput' })
go vet @packages
go vet -unsafeptr=false ./internal/wininput

New-Item -ItemType Directory -Force build | Out-Null
go build -trimpath -o build/krasis-lab.exe ./cmd/krasis-lab
go build -trimpath -o build/krasis-launcher.exe ./cmd/krasis-launcher
```

변경 중에는 `go test ./internal/control`, `go test ./internal/application`처럼 affected package만 먼저
돌린다. PR CI가 formatting, test, vet, 두 executable build, probe-kit verification과 artifact upload를
완료하므로 사소한 수정마다 전체 suite를 반복하지 않는다.

## Phase 0 실행물

물리 장비 작업은 `docs/experiments/phase-0-windows-input-feasibility.md`와 probe-kit의 `README.md`를
따른다. report는 사용자가 지정한 새 JSON 경로에만 쓰며 기존 파일을 덮어쓰지 않는다. repository에
report, certificate, private key, trust directory, 실제 host name·IP·경로를 commit하지 않는다.
장비 결과는 fixed alias `H-A`, `H-B`, `S-A`, `S-B`만 사용한다.

## 변경 규칙

상태 전이는 `internal/control/lease.go`, session 직렬화는 `SessionRuntime`, ordered write는 accepted ADR,
local trust persistence는 ADR 0006이 기준이다. adapter가 runtime state를 직접 바꾸거나 UI가 input hot
path를 소유하게 만들지 않는다. 새 unknown은 막연한 `UNDECIDED`로 남기지 말고 책임자, blocker와 결정
trigger를 문서에 기록한다.

## 생성물과 정리

`build/`, `dist/`, `.cache/go-build/`, smoke output과 내려받은 artifact는 언제든 재생성 가능한 작업
데이터다. source 수정 근거로 읽지 않는다. 실행 중인 probe나 필요한 evidence 전달이 끝난 뒤 삭제할 수
있다. `launcher-config.json`, `reports/`, `secrets/`는 repository output이 아니라 로컬 Phase 0 데이터이므로
`docs/desktop/local-data.md`의 보존·삭제 경계를 따른다.
