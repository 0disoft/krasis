Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$revision = (git rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $revision -notmatch '^[a-f0-9]{40}$' -or (git status --porcelain)) { throw 'Commit the source before building the revision-tagged trial' }
$runId = $revision.Substring(0, 12) + '-' + [DateTime]::UtcNow.ToString('yyyyMMddHHmmss')
$output = Join-Path (Get-Location) ('dist/uiaccess-input-probe/' + $runId)
if (Test-Path -LiteralPath $output) { throw 'Trial artifact already exists' }
$vswhere = 'C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe'
$vs = (& $vswhere -latest -products '*' -property installationPath).Trim()
if ($LASTEXITCODE -ne 0 -or -not $vs) { throw 'C++ build tools missing' }
$build = Join-Path $output 'build'
$objects = Join-Path $output 'obj'
New-Item -ItemType Directory -Path $build,$objects | Out-Null
& (Join-Path $vs 'MSBuild/Current/Bin/MSBuild.exe') experiments/system-input/system-trial.vcxproj /nologo /v:minimal /p:Configuration=Release /p:Platform=x64 "/p:OutDir=$build\" "/p:IntDir=$objects\"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$binary = Join-Path $build 'system-trial.exe'
$mt = 'C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\mt.exe'
$manifest = Join-Path $output 'embedded-manifest.xml'
& $mt "-inputresource:$binary;#1" "-out:$manifest"
if ($LASTEXITCODE -ne 0) { throw 'Manifest extraction failed' }
$xml = Get-Content -Raw -LiteralPath $manifest
if ($xml -notmatch 'level="asInvoker"' -or $xml -notmatch 'uiAccess="false"') { throw 'Service bootstrap must not request manifest UIAccess' }
# Keep the existing atomic six-file transport shape, compatible with g90+.
Copy-Item -LiteralPath $binary -Destination (Join-Path $output 'KrasisUIAccessProbe-UNSIGNED.exe')
foreach ($name in @('README.md','install-test.ps1','remove-test.ps1')) {
    Copy-Item -LiteralPath (Join-Path 'experiments/system-input' $name) -Destination (Join-Path $output $name)
}
Write-Output "Unsigned SYSTEM trial built: $output"
Write-Output 'No service, process or certificate was installed or started.'
