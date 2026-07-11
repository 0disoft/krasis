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
    [string]$OutputPath = (Join-Path (Split-Path -Parent $PSScriptRoot) 'dist/krasis-probe-kit-windows-amd64.zip')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
    throw "Probe binary not found: $BinaryPath"
}
if (Test-Path -LiteralPath $OutputPath) {
    throw "Package output already exists: $OutputPath"
}

$outputDirectory = Split-Path -Parent $OutputPath
$null = New-Item -ItemType Directory -Force -Path $outputDirectory
$stagingRoot = Join-Path $outputDirectory ('.krasis-probe-kit-' + [Guid]::NewGuid().ToString('N'))
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)

try {
    $null = New-Item -ItemType Directory -Path $stagingRoot
    Copy-Item -LiteralPath $BinaryPath -Destination (Join-Path $stagingRoot 'krasis-lab.exe')
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'collect-probe.ps1') -Destination $stagingRoot
    Copy-Item -LiteralPath (Join-Path $projectRoot 'docs/experiments/device-probe-runbook.md') -Destination (Join-Path $stagingRoot 'README.md')

    $executableHash = (Get-FileHash -LiteralPath (Join-Path $stagingRoot 'krasis-lab.exe') -Algorithm SHA256).Hash.ToLowerInvariant()
    [System.IO.File]::WriteAllText(
        (Join-Path $stagingRoot 'SHA256SUMS.txt'),
        "$executableHash  krasis-lab.exe`n",
        $utf8NoBom
    )
    $buildInfo = [ordered]@{
        schemaVersion = 'krasis.build/v1'
        commit = $Commit
        repository = $Repository
        workflowRun = $WorkflowRun
    } | ConvertTo-Json
    [System.IO.File]::WriteAllText(
        (Join-Path $stagingRoot 'BUILD.json'),
        "$buildInfo`n",
        $utf8NoBom
    )

    Compress-Archive -Path (Join-Path $stagingRoot '*') -DestinationPath $OutputPath
}
finally {
    if (Test-Path -LiteralPath $stagingRoot) {
        Remove-Item -LiteralPath $stagingRoot -Recurse -Force
    }
}

Write-Output $OutputPath
