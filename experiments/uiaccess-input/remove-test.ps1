param([Parameter(Mandatory = $true)][string]$Thumbprint)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run from an administrator PowerShell on B' }
if ($Thumbprint -notmatch '^[a-fA-F0-9]{40}$') { throw 'Expected exact certificate SHA-1 thumbprint' }
$thumbprint = $Thumbprint.ToUpperInvariant()
$directory = Join-Path $env:ProgramFiles 'KrasisUIAccessProbe'
$installed = Join-Path $directory 'KrasisUIAccessProbe.exe'
if (Test-Path -LiteralPath $installed) {
    $signature = Get-AuthenticodeSignature -LiteralPath $installed
    if (-not $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint -ne $thumbprint) {
        throw 'Installed probe signer differs from the requested certificate'
    }
    Remove-Item -LiteralPath $installed -Force
}
if (Test-Path -LiteralPath $directory) { Remove-Item -LiteralPath $directory -Force }
foreach ($store in @('TrustedPublisher', 'Root')) {
    $path = 'Cert:\LocalMachine\' + $store + '\' + $thumbprint
    if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force }
}
Write-Output 'Exact test probe and certificate removed.'
