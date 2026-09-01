[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$Commit,

    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$Repository,

    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$WorkflowRun,

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$BinaryPath = (Join-Path (Split-Path -Parent $PSScriptRoot) 'build/krasis-lab.exe'),

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$LauncherPath = (Join-Path (Split-Path -Parent $PSScriptRoot) 'build/krasis-launcher.exe'),

    [Parameter()]
    [string]$OutputPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-Sha256 {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Assert-ProbeKitArchiveMatches {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ArchivePath,

        [Parameter(Mandatory = $true)]
        [string]$ExpectedCommit,

        [Parameter(Mandatory = $true)]
        [string]$ExpectedManifestPath,

        [Parameter(Mandatory = $true)]
        [string]$TemporaryParent
    )

    $verificationRoot = Join-Path $TemporaryParent ('.krasis-probe-kit-verify-' + [Guid]::NewGuid().ToString('N'))
    try {
        $null = New-Item -ItemType Directory -Path $verificationRoot
        Expand-Archive -LiteralPath $ArchivePath -DestinationPath $verificationRoot

        $verifier = Join-Path $verificationRoot 'verify-kit.ps1'
        if (-not (Test-Path -LiteralPath $verifier -PathType Leaf)) {
            throw "Existing package does not contain verify-kit.ps1: $ArchivePath"
        }
        & $verifier | Out-Null

        $buildPath = Join-Path $verificationRoot 'BUILD.json'
        $build = Get-Content -LiteralPath $buildPath -Raw | ConvertFrom-Json
        if ($build.schemaVersion -ne 'krasis.build/v1') {
            throw "Existing package has an unsupported BUILD.json schema: $ArchivePath"
        }
        if ([string]$build.commit -ne $ExpectedCommit) {
            throw "Existing package commit does not match the requested revision: $ArchivePath"
        }

        $existingManifestPath = Join-Path $verificationRoot 'SHA256SUMS.txt'
        if ((Get-Sha256 -Path $existingManifestPath) -ne (Get-Sha256 -Path $ExpectedManifestPath)) {
            throw "Package output exists but does not match the current build payload: $ArchivePath"
        }
    }
    finally {
        if (Test-Path -LiteralPath $verificationRoot) {
            Remove-Item -LiteralPath $verificationRoot -Recurse -Force
        }
    }
}

$projectRoot = Split-Path -Parent $PSScriptRoot
if ($Commit -notmatch '^[0-9a-fA-F]{40}$') {
    throw 'Commit must be a full 40-character Git revision.'
}
$normalizedCommit = $Commit.ToLowerInvariant()

$headCommit = (& git -C $projectRoot rev-parse --verify HEAD 2>$null)
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($headCommit)) {
    throw 'Unable to resolve the current Git revision.'
}
if ($headCommit.Trim().ToLowerInvariant() -ne $normalizedCommit) {
    throw "Requested commit does not match the checked-out revision: $normalizedCommit"
}
$trackedChanges = @(& git -C $projectRoot status --porcelain --untracked-files=no 2>$null)
if ($LASTEXITCODE -ne 0) {
    throw 'Unable to inspect the Git working tree.'
}
if ($trackedChanges.Count -gt 0) {
    throw 'Probe kits must be packaged from a clean tracked working tree.'
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $projectRoot "dist/krasis-probe-kit-windows-amd64-$normalizedCommit.zip"
}
elseif (-not [System.IO.Path]::IsPathRooted($OutputPath)) {
    $OutputPath = Join-Path $projectRoot $OutputPath
}
$OutputPath = [System.IO.Path]::GetFullPath($OutputPath)

if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
    throw "Probe binary not found: $BinaryPath"
}
if (-not (Test-Path -LiteralPath $LauncherPath -PathType Leaf)) {
    throw "Launcher binary not found: $LauncherPath"
}

$outputDirectory = Split-Path -Parent $OutputPath
$null = New-Item -ItemType Directory -Force -Path $outputDirectory
$stagingRoot = Join-Path $outputDirectory ('.krasis-probe-kit-' + [Guid]::NewGuid().ToString('N'))
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)

try {
    $null = New-Item -ItemType Directory -Path $stagingRoot
    Copy-Item -LiteralPath $BinaryPath -Destination (Join-Path $stagingRoot 'krasis-lab.exe')
    Copy-Item -LiteralPath $LauncherPath -Destination (Join-Path $stagingRoot 'krasis-launcher.exe')
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'collect-probe.ps1') -Destination $stagingRoot
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'check-phase0.ps1') -Destination $stagingRoot
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'verify-kit.ps1') -Destination $stagingRoot
    Copy-Item -LiteralPath (Join-Path $projectRoot 'docs/experiments/device-probe-runbook.md') -Destination (Join-Path $stagingRoot 'README.md')

    $buildInfo = [ordered]@{
        schemaVersion = 'krasis.build/v1'
        commit = $normalizedCommit
        repository = $Repository
        workflowRun = $WorkflowRun
    } | ConvertTo-Json
    [System.IO.File]::WriteAllText(
        (Join-Path $stagingRoot 'BUILD.json'),
        "$buildInfo`n",
        $utf8NoBom
    )

    $checksumFiles = @(
        'krasis-lab.exe'
        'krasis-launcher.exe'
        'collect-probe.ps1'
        'check-phase0.ps1'
        'verify-kit.ps1'
        'README.md'
        'BUILD.json'
    )
    $checksumLines = foreach ($name in $checksumFiles) {
        $hash = Get-Sha256 -Path (Join-Path $stagingRoot $name)
        "$hash  $name"
    }
    $manifestPath = Join-Path $stagingRoot 'SHA256SUMS.txt'
    [System.IO.File]::WriteAllText(
        $manifestPath,
        (($checksumLines -join "`n") + "`n"),
        $utf8NoBom
    )

    $verifier = Join-Path $stagingRoot 'verify-kit.ps1'
    $unexpected = Join-Path $stagingRoot 'UNEXPECTED.txt'
    [System.IO.File]::WriteAllText($unexpected, "probe kit verifier negative check`n", $utf8NoBom)
    $unexpectedRejected = $false
    try {
        & $verifier | Out-Null
    }
    catch {
        $unexpectedRejected = $true
    }
    finally {
        Remove-Item -LiteralPath $unexpected -Force
    }
    if (-not $unexpectedRejected) {
        throw 'Probe kit verifier accepted an unexpected file.'
    }
    & $verifier | Out-Null

    if (Test-Path -LiteralPath $OutputPath) {
        if (-not (Test-Path -LiteralPath $OutputPath -PathType Leaf)) {
            throw "Package output path is not a file: $OutputPath"
        }
        Assert-ProbeKitArchiveMatches `
            -ArchivePath $OutputPath `
            -ExpectedCommit $normalizedCommit `
            -ExpectedManifestPath $manifestPath `
            -TemporaryParent $outputDirectory
        Write-Output $OutputPath
        return
    }

    Compress-Archive -Path (Join-Path $stagingRoot '*') -DestinationPath $OutputPath
    Assert-ProbeKitArchiveMatches `
        -ArchivePath $OutputPath `
        -ExpectedCommit $normalizedCommit `
        -ExpectedManifestPath $manifestPath `
        -TemporaryParent $outputDirectory
}
finally {
    if (Test-Path -LiteralPath $stagingRoot) {
        Remove-Item -LiteralPath $stagingRoot -Recurse -Force
    }
}

Write-Output $OutputPath
