# Krasis development

- Status: Active
- Owner: repository maintainers

## Toolchain

The Go version is declared in `go.mod`. PowerShell 7 is required for probe-kit scripts. Windows is required to build and exercise the Win32 input, launcher, lifecycle, credential-permission, and path-security implementations.

The `toolchain` directive selects Go 1.27.3 or a newer installed toolchain by default;
CI and release jobs select the toolchain reviewed in `scripts/runtime/stw-retake.json`
while the temporary runtime backport is required. Promote a stable patch by validating
its original sources, updating that manifest and `go.mod`, and passing the runtime
regression checks. Remove this selection exception together with the backport once
the upstream fix passes those checks, then resume following stable patches.
Go 1.27.2 fixed a Windows
[`LockOSThread` stack-corruption risk](https://github.com/golang/go/commit/87b06e2c18bf96180ff1cdd1f285cb18768d39d0)
on paths used by the input workers. This is not proof that it caused the recorded
g448 stalls. Keep automatic toolchain selection enabled for local builds.
Before offline workspace packaging, populate its module cache with
`$env:GOMODCACHE = Join-Path (Get-Location) '.cache/go-mod'; go mod download`.

Non-Windows hosts can work on pure state, transport, trust-store, report-schema, and application packages, but they cannot claim Windows behavior or physical-device acceptance.

## Repository entry points

- `cmd/krasis-lab`: bounded Phase 0 diagnostic and peer-session CLI
- `cmd/krasis-launcher`: native Windows launcher for the probe workflow
- `internal/control`: deterministic lease, sequence, liveness, and pressed-input model
- `internal/application`: serialized runtime owner and effect execution
- `internal/peer` and `internal/transport`: authenticated ordered peer session
- `internal/wininput`: Windows capture, suppression, boundary, and injection adapters
- `internal/agent`: session supervisor and lifecycle integration
- `internal/truststore` and `internal/peercredentials`: Phase 0 trust and credential storage
- `docs/experiments`: physical-device gates and evidence contracts

## Local validation

Run formatting first:

```powershell
$goFiles = @(Get-ChildItem -Recurse -File -Filter '*.go' | Select-Object -ExpandProperty FullName)
$unformatted = @($goFiles | ForEach-Object { gofmt -l $_ })
if ($unformatted.Count -gt 0) { $unformatted; exit 1 }
```

Run deterministic tests:

```powershell
go test ./...
```

Run vet with the repository's narrow Win32 exception:

```powershell
$packages = @(go list ./... | Where-Object { $_ -ne 'github.com/0disoft/krasis/internal/wininput' })
go vet @packages
go vet -unsafeptr=false ./internal/wininput
```

Build the Windows probe executables:

```powershell
New-Item -ItemType Directory -Force build | Out-Null
go build -trimpath -o build/krasis-lab.exe ./cmd/krasis-lab
go build -trimpath -o build/krasis-launcher.exe ./cmd/krasis-launcher
```

Package only a clean, exact revision and verify the resulting archive with `scripts/package-probe-kit.ps1` and `scripts/verify-kit.ps1`. Never overwrite a revision-addressed evidence artifact with different bytes.

## Test boundaries

CI proves buildability and deterministic contracts on `windows-latest`. It does not prove touchpad behavior, laptop-specific capture and suppression, suspend or lock recovery, real network interruption recovery, latency, or soak stability. Those claims require the physical procedures and accepted aggregate reports in `docs/experiments/`.

## Generated and local output

`build/`, `dist/`, temporary reports, credentials, and local configuration are not source truth and must remain untracked. Do not use local absolute paths or private machine data in tests, docs, commit messages, or pull requests.
