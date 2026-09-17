Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$env:GOMODCACHE = Join-Path (Get-Location) '.cache/go-mod'
$env:GOCACHE = Join-Path (Get-Location) '.cache/go-build'
$env:GOPROXY = 'off'
$env:CGO_ENABLED = '0'
gofmt -w cmd/krasis-workspace/main_windows.go cmd/krasis-workspace/service_windows.go assets/windows/embed.go internal/productpanel/desktop_lifecycle.go internal/productpanel/desktop_lifecycle_test.go
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm.cmd --prefix frontend test
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm.cmd --prefix frontend run build -- --mode native
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go build -tags desktop,production -ldflags '-H windowsgui' -o dist/workspace/krasis-workspace.exe ./cmd/krasis-workspace
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go build -ldflags '-H windowsgui' -o dist/workspace/krasis-agent.exe ./cmd/krasis-agent
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$files = @('krasis-workspace.exe', 'krasis-agent.exe')
$sums = foreach ($file in $files) { (Get-FileHash -LiteralPath (Join-Path 'dist/workspace' $file) -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $file }
[IO.File]::WriteAllLines((Join-Path (Get-Location) 'dist/workspace/SHA256SUMS.txt'), $sums, [Text.UTF8Encoding]::new($false))
Copy-Item -LiteralPath 'frontend/README.md' -Destination 'dist/workspace/START-HERE.md'
$notice = [Collections.Generic.List[string]]::new()
foreach ($line in Get-Content -LiteralPath 'go.mod') {
 if ($line -match '^\s*(\S+)\s+(v\S+)') {
  $module = $Matches[1]; $version = $Matches[2]
  $directory = Join-Path '.cache/go-mod' ($module + '@' + $version)
  foreach ($license in Get-ChildItem -LiteralPath $directory -File | Where-Object {$_.Name -match '^(LICENSE|COPYING|NOTICE)'}) {
   $notice.Add($module + ' ' + $version)
   $notice.Add([IO.File]::ReadAllText($license.FullName))
  }
 }
}
[IO.File]::WriteAllLines((Join-Path (Get-Location) 'dist/workspace/THIRD-PARTY-NOTICES.txt'), $notice, [Text.UTF8Encoding]::new($false))
$files += @('SHA256SUMS.txt','START-HERE.md','THIRD-PARTY-NOTICES.txt')
Compress-Archive -LiteralPath ($files | ForEach-Object {Join-Path 'dist/workspace' $_}) -DestinationPath 'dist/workspace/krasis-workspace-test.zip' -Force
Write-Output 'Workspace and verified sibling agent packaged; no installed process was started or changed.'
