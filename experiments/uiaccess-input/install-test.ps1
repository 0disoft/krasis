param(
    [Parameter(Mandatory = $true)][string]$ArtifactDirectory,
    [Parameter(Mandatory = $true)][string]$ExpectedSHA256,
    [Parameter(Mandatory = $true)][string]$ExpectedThumbprint
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run from an administrator PowerShell on B' }
$artifact = (Resolve-Path -LiteralPath $ArtifactDirectory).Path
$exe = Join-Path $artifact 'KrasisUIAccessProbe-TESTSIGNED.exe'
$certFile = Join-Path $artifact 'KrasisUIAccessProbe-TESTCERT.cer'
$detailsFile = Join-Path $artifact 'TEST-CERT-DETAILS.txt'
foreach ($path in @($exe, $certFile, $detailsFile)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Missing probe artifact: $path" }
}
$details = @{}
Get-Content -LiteralPath $detailsFile | ForEach-Object {
    $parts = $_ -split ': ', 2
    if ($parts.Count -eq 2) { $details[$parts[0]] = $parts[1] }
}
$certificate = [Security.Cryptography.X509Certificates.X509Certificate2]::new($certFile)
$thumbprint = $certificate.Thumbprint.ToUpperInvariant()
$hash = (Get-FileHash -LiteralPath $exe -Algorithm SHA256).Hash.ToLowerInvariant()
$signature = Get-AuthenticodeSignature -LiteralPath $exe
if ($ExpectedSHA256 -notmatch '^[a-fA-F0-9]{64}$' -or $ExpectedThumbprint -notmatch '^[a-fA-F0-9]{40}$' -or
    $thumbprint -ne $ExpectedThumbprint.ToUpperInvariant() -or $hash -ne $ExpectedSHA256.ToLowerInvariant() -or
    $thumbprint -ne $details.Thumbprint -or $hash -ne $details['EXE SHA256'] -or
    -not $signature.SignerCertificate -or $signature.SignerCertificate.Thumbprint -ne $thumbprint -or
    $certificate.NotAfter -le (Get-Date) -or $certificate.HasPrivateKey) {
    throw 'Probe signature, certificate, hash, or expiry does not match the build record'
}
$directory = Join-Path $env:ProgramFiles 'KrasisUIAccessProbe'
$installed = Join-Path $directory 'KrasisUIAccessProbe.exe'
if (Test-Path -LiteralPath $directory) { throw 'Probe installation already exists; clean it up first' }
$root = 'Cert:\LocalMachine\Root\' + $thumbprint
$publisher = 'Cert:\LocalMachine\TrustedPublisher\' + $thumbprint
if ((Test-Path -LiteralPath $root) -or (Test-Path -LiteralPath $publisher)) {
    throw 'The test certificate was already trusted; refusing to change existing trust'
}

try {
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
    Copy-Item -LiteralPath $exe -Destination $installed
    Import-Certificate -FilePath $certFile -CertStoreLocation 'Cert:\LocalMachine\Root' | Out-Null
    Import-Certificate -FilePath $certFile -CertStoreLocation 'Cert:\LocalMachine\TrustedPublisher' | Out-Null
    if ((Get-AuthenticodeSignature -LiteralPath $installed).Status -ne 'Valid') {
        throw 'Installed probe signature is not valid'
    }
    Write-Output "Installed test probe: $installed"
    Write-Output "Trusted test certificate: $thumbprint"
    Write-Output 'Close this elevated PowerShell. Launch the EXE from a normal Explorer window on B.'
} catch {
    if (Test-Path -LiteralPath $installed) { Remove-Item -LiteralPath $installed -Force }
    if (Test-Path -LiteralPath $directory) { Remove-Item -LiteralPath $directory -Force }
    foreach ($store in @($publisher, $root)) {
        if (Test-Path -LiteralPath $store) { Remove-Item -LiteralPath $store -Force }
    }
    throw
}
