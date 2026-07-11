[CmdletBinding()]
param(
    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$ReportDirectory = (Join-Path $PSScriptRoot 'reports'),

    [Parameter()]
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$executable = Join-Path $PSScriptRoot 'krasis-lab.exe'
if (-not (Test-Path -LiteralPath $executable -PathType Leaf)) {
    throw 'krasis-lab.exe was not found next to this script. Extract the complete probe kit first.'
}
if (-not (Test-Path -LiteralPath $ReportDirectory -PathType Container)) {
    throw "Report directory was not found: $ReportDirectory"
}

$directory = (Resolve-Path -LiteralPath $ReportDirectory).Path
$names = [ordered]@{
    HomeA = 'H-A-agent.json'
    HomeB = 'H-B-agent.json'
    StudyA = 'S-A-agent.json'
    StudyB = 'S-B-agent.json'
    RecoveryHomeA = 'H-A-recovery.json'
    RecoveryHomeB = 'H-B-recovery.json'
    RecoveryStudyA = 'S-A-recovery.json'
    RecoveryStudyB = 'S-B-recovery.json'
}
$paths = @{}
$missing = [System.Collections.Generic.List[string]]::new()
foreach ($entry in $names.GetEnumerator()) {
    $path = Join-Path $directory $entry.Value
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        $missing.Add($entry.Value)
    }
    $paths[$entry.Key] = $path
}
if ($missing.Count -ne 0) {
    throw ('Missing Phase 0 reports: ' + ($missing -join ', '))
}

$arguments = @(
    'phase0-check'
    '--home'; $paths.HomeA
    '--home'; $paths.HomeB
    '--study'; $paths.StudyA
    '--study'; $paths.StudyB
    '--recovery'; $paths.RecoveryHomeA
    '--recovery'; $paths.RecoveryHomeB
    '--recovery'; $paths.RecoveryStudyA
    '--recovery'; $paths.RecoveryStudyB
)
if ($Json) {
    $arguments += '--json'
}

& $executable @arguments
$exitCode = $LASTEXITCODE
if ($null -eq $exitCode) {
    throw 'krasis-lab.exe did not return a native process exit code.'
}
exit $exitCode
