Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Load only the verifier functions; do not run the packaging/build entrypoint.
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    (Join-Path $PSScriptRoot 'package-probe-kit.ps1'), [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -ne 0) { throw $parseErrors[0] }
foreach ($name in @('Get-Sha256', 'Assert-ProbeKitArchiveMatches')) {
    $function = $ast.Find({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $true)
    if ($null -eq $function) { throw "Missing function: $name" }
    . ([scriptblock]::Create($function.Extent.Text))
}

$temp = Join-Path ([IO.Path]::GetTempPath()) ('krasis-kit-test-' + [Guid]::NewGuid().ToString('N'))
try {
    $source = Join-Path $temp 'source'
    $null = New-Item -ItemType Directory -Path $source -Force
    $commit = 'a' * 40
    $marker = Join-Path $temp 'executed.txt'
    $malicious = "Set-Content -LiteralPath '$marker' -Value executed"
    $names = @('krasis-lab.exe', 'krasis-launcher.exe', 'collect-probe.ps1', 'check-phase0.ps1', 'verify-kit.ps1', 'README.md', 'BUILD.json')
    foreach ($name in $names) {
        $content = if ($name -eq 'BUILD.json') { "{`"schemaVersion`":`"krasis.build/v1`",`"commit`":`"$commit`"}" } elseif ($name -eq 'verify-kit.ps1') { 'x' * $malicious.Length } else { "fixture $name" }
        [IO.File]::WriteAllText((Join-Path $source $name), $content)
    }
    $manifest = Join-Path $source 'SHA256SUMS.txt'
    $lines = foreach ($name in $names) { "$(Get-Sha256 -Path (Join-Path $source $name))  $name" }
    [IO.File]::WriteAllLines($manifest, [string[]]$lines)
    $reference = Join-Path $temp 'reference'
    Copy-Item -LiteralPath $source -Destination $reference -Recurse
    $manifest = Join-Path $reference 'SHA256SUMS.txt'
    $archive = Join-Path $temp 'valid.zip'
    Compress-Archive -Path (Join-Path $source '*') -DestinationPath $archive
    Assert-ProbeKitArchiveMatches -ArchivePath $archive -ExpectedCommit $commit -ExpectedManifestPath $manifest -TemporaryParent $temp

    [IO.File]::WriteAllText((Join-Path $source 'verify-kit.ps1'), $malicious)
    $archive = Join-Path $temp 'tampered.zip'
    Compress-Archive -Path (Join-Path $source '*') -DestinationPath $archive
    $rejected = $false
    try {
        Assert-ProbeKitArchiveMatches -ArchivePath $archive -ExpectedCommit $commit -ExpectedManifestPath $manifest -TemporaryParent $temp
    } catch { $rejected = $true }
    if (-not $rejected -or (Test-Path -LiteralPath $marker)) {
        throw 'Tampered verifier was accepted or executed.'
    }
    Copy-Item -LiteralPath (Join-Path $reference 'verify-kit.ps1') -Destination (Join-Path $source 'verify-kit.ps1') -Force
    [IO.File]::WriteAllText((Join-Path $source 'README.md'), ('oversized' * 1000))
    $archive = Join-Path $temp 'oversized.zip'
    Compress-Archive -Path (Join-Path $source '*') -DestinationPath $archive
    $rejected = $false
    try {
        Assert-ProbeKitArchiveMatches -ArchivePath $archive -ExpectedCommit $commit -ExpectedManifestPath $manifest -TemporaryParent $temp
    } catch { $rejected = $true }
    if (-not $rejected) { throw 'Oversized archive entry was accepted.' }
} finally {
    if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Recurse -Force }
}
Write-Output 'Probe kit archive verification passed.'
