Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$revision = (git rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $revision -notmatch '^[a-f0-9]{40}$') { throw 'Missing source revision' }
if (git status --porcelain) { throw 'Commit source changes before building the revision-tagged probe' }
$runId = $revision.Substring(0, 12) + '-' + [DateTime]::UtcNow.ToString('yyyyMMddHHmmss')
$output = Join-Path (Get-Location) ('dist/uiaccess-input-probe/' + $runId)
if (Test-Path -LiteralPath $output) { throw 'Probe artifact already exists' }
$vswhere = 'C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe'
if (-not (Test-Path -LiteralPath $vswhere)) { throw 'Visual Studio locator missing' }
$vs = (& $vswhere -latest -products '*' -property installationPath).Trim()
if ($LASTEXITCODE -ne 0 -or -not $vs) { throw 'Visual Studio C++ workload unavailable' }
$msbuild = Join-Path $vs 'MSBuild/Current/Bin/MSBuild.exe'
$mt = 'C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\mt.exe'
if (-not (Test-Path -LiteralPath $msbuild) -or -not (Test-Path -LiteralPath $mt)) {
    throw 'Required build tools unavailable'
}

$build = Join-Path $output 'build'
$objects = Join-Path $output 'obj'
New-Item -ItemType Directory -Path $build, $objects -Force | Out-Null
$project = Join-Path (Get-Location) 'experiments/uiaccess-input/uiaccess-probe.vcxproj'
& $msbuild $project /nologo /v:minimal /p:Configuration=Release /p:Platform=x64 "/p:OutDir=$build\" "/p:IntDir=$objects\"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$binary = Join-Path $build 'uiaccess-probe.exe'
if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { throw 'Probe executable missing' }
$manifest = Join-Path $output 'embedded-manifest.xml'
& $mt "-inputresource:$binary;#1" "-out:$manifest"
if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect embedded manifest' }
$content = Get-Content -Raw -LiteralPath $manifest
if ($content -notmatch 'level="asInvoker"' -or $content -notmatch 'uiAccess="true"') {
    throw 'Embedded UIAccess manifest is incorrect'
}
$unsigned = Join-Path $output 'KrasisUIAccessProbe-UNSIGNED.exe'
Copy-Item -LiteralPath $binary -Destination $unsigned
Copy-Item -LiteralPath 'experiments/uiaccess-input/README.md' -Destination (Join-Path $output 'README.md')
Copy-Item -LiteralPath 'experiments/uiaccess-input/install-test.ps1' -Destination (Join-Path $output 'install-test.ps1')
Copy-Item -LiteralPath 'experiments/uiaccess-input/remove-test.ps1' -Destination (Join-Path $output 'remove-test.ps1')
$hash = (Get-FileHash -LiteralPath $unsigned -Algorithm SHA256).Hash.ToLowerInvariant()
Write-Output "Unsigned local-only UIAccess probe built: $unsigned"
Write-Output "SHA256: $hash"
