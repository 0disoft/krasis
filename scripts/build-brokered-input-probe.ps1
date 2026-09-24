Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$revision = (git rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $revision -notmatch '^[a-f0-9]{40}$') { throw 'Missing source revision' }
$runId = $revision.Substring(0, 12) + '-' + [DateTime]::UtcNow.ToString('yyyyMMddHHmmss')
$output = Join-Path (Get-Location) ('dist/brokered-input-probe/' + $runId)
if (Test-Path -LiteralPath $output) { throw 'Probe artifact already exists' }
$source = Join-Path (Get-Location) 'experiments/brokered-input'
$project = Join-Path $source 'brokered-probe.vcxproj'
$vswhere = 'C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe'
if (-not (Test-Path -LiteralPath $vswhere)) { throw 'Visual Studio locator missing' }
$vs = (& $vswhere -latest -products '*' -property installationPath).Trim()
if ($LASTEXITCODE -ne 0 -or -not $vs) { throw 'Visual Studio C++ workload unavailable' }
$msbuild = Join-Path $vs 'MSBuild/Current/Bin/MSBuild.exe'
$makeappx = 'C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\makeappx.exe'
if (-not (Test-Path -LiteralPath $msbuild) -or -not (Test-Path -LiteralPath $makeappx)) {
    throw 'Required build tools unavailable'
}

$build = Join-Path $output 'build'
$objects = Join-Path $output 'obj'
$stage = Join-Path $output 'stage'
New-Item -ItemType Directory -Path $build, $objects, (Join-Path $stage 'Assets') -Force | Out-Null
& $msbuild $project /nologo /v:minimal /p:Configuration=Release /p:Platform=x64 "/p:OutDir=$build\" "/p:IntDir=$objects\"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$binary = Join-Path $build 'brokered-probe.exe'
if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { throw 'Probe executable missing' }
Copy-Item -LiteralPath $binary -Destination (Join-Path $stage 'KrasisBrokeredProbe.exe')
Copy-Item -LiteralPath (Join-Path $source 'AppxManifest.xml') -Destination (Join-Path $stage 'AppxManifest.xml')

Add-Type -AssemblyName System.Drawing
$icon = [System.Drawing.Image]::FromFile((Join-Path (Get-Location) 'assets/windows/krasis-source.png'))
try {
    foreach ($size in @(44, 150)) {
        $bitmap = [System.Drawing.Bitmap]::new($size, $size)
        try {
            $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
            try {
                $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
                $graphics.DrawImage($icon, 0, 0, $size, $size)
            } finally { $graphics.Dispose() }
            $bitmap.Save((Join-Path $stage "Assets/Logo$size.png"), [System.Drawing.Imaging.ImageFormat]::Png)
        } finally { $bitmap.Dispose() }
    }
} finally { $icon.Dispose() }

$package = Join-Path $output 'KrasisBrokeredProbe-UNSIGNED.msix'
& $makeappx pack /d $stage /p $package /o
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Copy-Item -LiteralPath (Join-Path $source 'README.md') -Destination (Join-Path $output 'README.md')
$hash = (Get-FileHash -LiteralPath $package -Algorithm SHA256).Hash.ToLowerInvariant()
Write-Output "Unsigned local-only input probe built: $package"
Write-Output "SHA256: $hash"
