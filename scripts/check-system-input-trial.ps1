Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
foreach ($file in @('experiments/system-input/install-test.ps1','experiments/system-input/remove-test.ps1','scripts/build-system-input-trial.ps1')) {
    $tokens = $null; $errors = $null
    [Management.Automation.Language.Parser]::ParseFile((Join-Path (Get-Location) $file), [ref]$tokens, [ref]$errors) | Out-Null
    if ($errors.Count) { $errors; throw 'PowerShell syntax errors' }
}
$vs = (& 'C:/Program Files (x86)/Microsoft Visual Studio/Installer/vswhere.exe' -latest -products '*' -property installationPath).Trim()
$root = Join-Path (Get-Location) '.cache/system-manifest-check'
$build = Join-Path $root 'build'; $objects = Join-Path $root 'obj'
New-Item -ItemType Directory -Path $build,$objects -Force | Out-Null
& (Join-Path $vs 'MSBuild/Current/Bin/MSBuild.exe') experiments/system-input/system-trial.vcxproj /nologo /v:minimal /p:Configuration=Release /p:Platform=x64 "/p:OutDir=$build\" "/p:IntDir=$objects\"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$binary = Join-Path $build 'system-trial.exe'
$manifest = Join-Path $root 'embedded-manifest.xml'
& 'C:/Program Files (x86)/Windows Kits/10/bin/10.0.26100.0/x64/mt.exe' "-inputresource:$binary;#1" "-out:$manifest"
if ($LASTEXITCODE -ne 0) { throw 'Manifest extraction failed' }
$xml = Get-Content -Raw -LiteralPath $manifest
if ($xml -notmatch 'level="asInvoker"' -or $xml -notmatch 'uiAccess="false"') { throw 'Invalid service bootstrap manifest' }
# Exercise process creation, not a service or an input trial. This cache path
# cannot equal the hard-coded Program Files installation, so service/worker
# invocations must deny execution with code 12. The separate queue smoke creates
# a never-shown window with an unconditional denial callback and sends no input.
foreach ($case in @(
    @{Argument='--service'; Expected=12}, @{Argument='--worker'; Expected=12},
    @{Argument='--invalid'; Expected=12}, @{Argument='--queue-refusal-check'; Expected=0},
    @{Argument='--token-evidence-check'; Expected=0}, @{Argument='--desktop-access-check'; Expected=0}
)) {
    $argument = $case.Argument
    $process = [Diagnostics.Process]::new()
    try {
        $process.StartInfo.FileName = $binary
        $process.StartInfo.Arguments = $argument
        $process.StartInfo.WorkingDirectory = $build
        $process.StartInfo.UseShellExecute = $false
        $process.StartInfo.CreateNoWindow = $true
        if (-not $process.Start()) { throw 'Process creation failed' }
        if (-not $process.WaitForExit(5000)) { $process.Kill(); $process.WaitForExit(); throw 'Refusal probe timed out' }
        if ($process.ExitCode -ne $case.Expected) { throw "Trial check failed for $argument : $($process.ExitCode)" }
        Write-Output "Process creation and bounded refusal check passed: $argument ($($case.Expected))"
    } finally { $process.Dispose() }
}
git diff --check
exit $LASTEXITCODE
