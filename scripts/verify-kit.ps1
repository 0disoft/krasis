[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$manifestPath = Join-Path $PSScriptRoot 'SHA256SUMS.txt'
if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
    throw 'SHA256SUMS.txt was not found next to this script.'
}

$requiredFiles = @(
    'krasis-lab.exe'
    'krasis-launcher.exe'
    'collect-probe.ps1'
    'check-phase0.ps1'
    'verify-kit.ps1'
    'README.md'
    'BUILD.json'
)
$allowedEntries = @($requiredFiles) + 'SHA256SUMS.txt'
foreach ($entry in Get-ChildItem -LiteralPath $PSScriptRoot -Force) {
    if ($entry.PSIsContainer -or $allowedEntries -notcontains $entry.Name) {
        throw "Unexpected probe kit entry: $($entry.Name)"
    }
}
foreach ($name in $allowedEntries) {
    if (-not (Test-Path -LiteralPath (Join-Path $PSScriptRoot $name) -PathType Leaf)) {
        throw "Required probe kit file was not found: $name"
    }
}

$expected = @{}
foreach ($line in Get-Content -LiteralPath $manifestPath) {
    if ($line -notmatch '^([0-9a-f]{64})  ([^/\\]+)$') {
        throw "Invalid checksum manifest line: $line"
    }
    $name = $Matches[2]
    if ($requiredFiles -notcontains $name) {
        throw "Unexpected checksum entry: $name"
    }
    if ($expected.ContainsKey($name)) {
        throw "Duplicate checksum entry: $name"
    }
    $expected[$name] = $Matches[1]
}

if ($expected.Count -ne $requiredFiles.Count) {
    $missing = @($requiredFiles | Where-Object { -not $expected.ContainsKey($_) })
    throw ('Missing checksum entries: ' + ($missing -join ', '))
}

foreach ($name in $requiredFiles) {
    $path = Join-Path $PSScriptRoot $name
    $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected[$name]) {
        throw "Checksum mismatch: $name"
    }
}

Write-Output 'Probe kit checksums verified.'
