Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$revision = (git rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $revision -notmatch '^[a-f0-9]{40}$') { throw 'Missing source revision' }
$root = Join-Path (Get-Location) 'dist/brokered-input-probe'
$candidate = Get-ChildItem -LiteralPath $root -Directory |
    Where-Object { $_.Name -like ($revision.Substring(0, 12) + '-*') } |
    Sort-Object LastWriteTime -Descending | Select-Object -First 1
if (-not $candidate) { throw 'Build the exact current revision first' }
$unsigned = Join-Path $candidate.FullName 'KrasisBrokeredProbe-UNSIGNED.msix'
$signed = Join-Path $candidate.FullName 'KrasisBrokeredProbe-TESTSIGNED.msix'
$public = Join-Path $candidate.FullName 'KrasisBrokeredProbe-TESTCERT.cer'
if (-not (Test-Path -LiteralPath $unsigned -PathType Leaf) -or
    (Test-Path -LiteralPath $signed) -or (Test-Path -LiteralPath $public)) {
    throw 'Expected one unsigned artifact and no previous signing output'
}
$signtool = 'C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\signtool.exe'
if (-not (Test-Path -LiteralPath $signtool)) { throw 'SignTool unavailable' }

$certificate = New-SelfSignedCertificate -Type CodeSigningCert `
    -Subject 'CN=KrasisDevelopment' -CertStoreLocation 'Cert:\CurrentUser\My' `
    -KeyAlgorithm RSA -KeyLength 3072 -HashAlgorithm SHA256 `
    -KeyExportPolicy NonExportable -NotAfter (Get-Date).AddDays(7)
if (-not $certificate -or $certificate.Subject -ne 'CN=KrasisDevelopment') {
    throw 'Test certificate creation failed'
}
try {
    Export-Certificate -Cert $certificate -FilePath $public -Type CERT | Out-Null
    Copy-Item -LiteralPath $unsigned -Destination $signed
    & $signtool sign /fd SHA256 /sha1 $certificate.Thumbprint /s My $signed
    if ($LASTEXITCODE -ne 0) { throw 'Test package signing failed' }
    $signature = Get-AuthenticodeSignature -LiteralPath $signed
    if (-not $signature.SignerCertificate -or
        $signature.SignerCertificate.Thumbprint -ne $certificate.Thumbprint) {
        throw 'Signed package certificate mismatch'
    }
    $hash = (Get-FileHash -LiteralPath $signed -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllLines((Join-Path $candidate.FullName 'TEST-CERT-DETAILS.txt'), @(
        'Subject: ' + $certificate.Subject,
        'Thumbprint: ' + $certificate.Thumbprint,
        'Expires UTC: ' + $certificate.NotAfter.ToUniversalTime().ToString('u'),
        'Package SHA256: ' + $hash
    ), [Text.UTF8Encoding]::new($false))
    Write-Output "Test-signed package: $signed"
    Write-Output "SHA256: $hash"
    Write-Output "Public test certificate: $public"
    Write-Output 'The certificate is not trusted or installed on the destination.'
} finally {
    $path = 'Cert:\CurrentUser\My\' + $certificate.Thumbprint
    if (Test-Path -LiteralPath $path) {
        Remove-Item -LiteralPath $path -DeleteKey -Force
    }
}
