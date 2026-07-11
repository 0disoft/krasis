[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('H-A', 'H-B', 'S-A', 'S-B')]
    [string]$DeviceAlias,

    [Parameter()]
    [ValidateSet('Basic', 'Guided')]
    [string]$Mode = 'Basic',

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$Duration = '30s',

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$StageDuration = '10s',

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$OutputDirectory = (Join-Path $PSScriptRoot 'reports')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$executable = Join-Path $PSScriptRoot 'krasis-lab.exe'
if (-not (Test-Path -LiteralPath $executable -PathType Leaf)) {
    throw "krasis-lab.exe was not found next to this script. Extract the complete probe kit first."
}

$null = New-Item -ItemType Directory -Force -Path $OutputDirectory
$resolvedOutputDirectory = (Resolve-Path -LiteralPath $OutputDirectory).Path
$timestamp = [DateTime]::UtcNow.ToString("yyyyMMdd'T'HHmmssfff'Z'")
$modeName = $Mode.ToLowerInvariant()
$outputPath = Join-Path $resolvedOutputDirectory "$DeviceAlias-$modeName-$timestamp.json"
if ($Mode -eq 'Guided') {
    $arguments = @(
        'guided-probe'
        '--device'
        $DeviceAlias
        '--stage-duration'
        $StageDuration
        '--output'
        $outputPath
    )
}
else {
    $arguments = @(
        'probe'
        '--device'
        $DeviceAlias
        '--duration'
        $Duration
        '--output'
        $outputPath
    )
}

& $executable @arguments
$exitCode = $LASTEXITCODE
if ($null -eq $exitCode) {
    throw 'krasis-lab.exe did not return a native process exit code.'
}
exit $exitCode
