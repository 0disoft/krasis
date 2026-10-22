param([switch]$Review)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$revision = (git rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $revision -notmatch '^[a-f0-9]{40}$') { throw 'Missing source revision' }
if (git status --porcelain --untracked-files=no) { throw 'Commit source changes before packaging' }
$generation = [regex]::Match((Get-Content -Raw -LiteralPath 'internal/localapi/runtime.go'), '(?m)^const AgentGeneration = ([1-9][0-9]*)\r?$')
if (-not $generation.Success) { throw 'Missing agent generation' }
$outputRoot = if ($Review) { Join-Path 'dist/review' $revision } else { 'dist/workspace' }
$bundle = Join-Path $outputRoot ('g' + $generation.Groups[1].Value)
if (Test-Path -LiteralPath $bundle) { throw 'Generation artifact already exists; do not overwrite running programs' }

$env:GOMODCACHE = Join-Path (Get-Location) '.cache/go-mod'
$env:GOCACHE = Join-Path (Get-Location) '.cache/go-build'
$env:GOPROXY = 'off'
$env:CGO_ENABLED = '0'
npm.cmd --prefix frontend test
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm.cmd --prefix frontend run build -- --mode native
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
New-Item -ItemType Directory -Path $bundle | Out-Null
Copy-Item -LiteralPath 'frontend/README.md' -Destination (Join-Path $bundle 'START-HERE.md')
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
[IO.File]::WriteAllLines((Join-Path $bundle 'THIRD-PARTY-NOTICES.txt'), $notice, [Text.UTF8Encoding]::new($false))
node scripts/build-workspace-update.mjs $bundle
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
node scripts/check-workspace-package.mjs $bundle
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Output "Krasis packaged: $bundle (UI, agent, CLI; installed processes unchanged)."
