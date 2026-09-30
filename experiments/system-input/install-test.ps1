param(
    [Parameter(Mandatory = $true)][string]$ArtifactDirectory,
    [Parameter(Mandatory = $true)][string]$ExpectedSHA256,
    [Parameter(Mandatory = $true)][string]$ExpectedThumbprint
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
if (-not ([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'B의 관리자 PowerShell에서 실행하세요.'
}
if ($ExpectedSHA256 -notmatch '^[a-fA-F0-9]{64}$' -or $ExpectedThumbprint -notmatch '^[a-fA-F0-9]{40}$') { throw 'A가 따로 전달한 해시와 지문이 필요합니다.' }
Add-Type -TypeDefinition @'
using System.Runtime.InteropServices;
public static class KrasisTrialConsole {
    [DllImport("kernel32.dll")] public static extern uint WTSGetActiveConsoleSessionId();
}
'@
$session = [Diagnostics.Process]::GetCurrentProcess().SessionId
if ($session -eq 0 -or $session -ne [KrasisTrialConsole]::WTSGetActiveConsoleSessionId()) { throw 'B의 현재 콘솔 세션에서만 설치할 수 있습니다.' }
$serviceName = 'KrasisSystemInputTrial'
$sc = Join-Path ([Environment]::GetFolderPath('System')) 'sc.exe'
$directory = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) $serviceName
$installed = Join-Path $directory ($serviceName + '.exe')
if ((Test-Path -LiteralPath $directory) -or (Get-Service -Name $serviceName -ErrorAction SilentlyContinue)) { throw '기존 시험 설치 또는 서비스가 있습니다. 먼저 제거하세요.' }
$artifact = (Resolve-Path -LiteralPath $ArtifactDirectory).Path
$bytes = [IO.File]::ReadAllBytes((Join-Path $artifact 'KrasisUIAccessProbe-TESTSIGNED.exe'))
$digest = [Security.Cryptography.SHA256]::Create()
try { $hash = ([BitConverter]::ToString($digest.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant() } finally { $digest.Dispose() }
$certificate = [Security.Cryptography.X509Certificates.X509Certificate2]::new([IO.File]::ReadAllBytes((Join-Path $artifact 'KrasisUIAccessProbe-TESTCERT.cer')))
$thumbprint = $certificate.Thumbprint.ToUpperInvariant()
if ($hash -ne $ExpectedSHA256.ToLowerInvariant() -or $thumbprint -ne $ExpectedThumbprint.ToUpperInvariant() -or
    $certificate.HasPrivateKey -or $certificate.Subject -ne 'CN=KrasisUIAccessExperiment' -or
    $certificate.NotBefore -gt (Get-Date) -or $certificate.NotAfter -le (Get-Date)) { throw 'EXE·인증서가 A의 전달값과 일치하지 않습니다.' }
$stores = @(('Cert:\LocalMachine\Root\' + $thumbprint), ('Cert:\LocalMachine\TrustedPublisher\' + $thumbprint))
foreach ($store in $stores) { if (Test-Path -LiteralPath $store) { throw '이미 신뢰된 인증서는 이 시험에서 변경하지 않습니다.' } }
$createdService = $false
$createdDirectory = $false
$trustedStores = [Collections.Generic.List[string]]::new()
try {
    New-Item -ItemType Directory -Path $directory | Out-Null
    $createdDirectory = $true
    $acl = [Security.AccessControl.DirectorySecurity]::new()
    $acl.SetSecurityDescriptorSddlForm('O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;BU)')
    Set-Acl -LiteralPath $directory -AclObject $acl
    [IO.File]::WriteAllBytes($installed, $bytes)
    # Trust exactly the pinned public certificate, never a private key or arbitrary chain.
    foreach ($storeName in @('Root', 'TrustedPublisher')) {
        $store = [Security.Cryptography.X509Certificates.X509Store]::new($storeName, 'LocalMachine')
        try { $store.Open('ReadWrite'); $store.Add($certificate); $trustedStores.Add('Cert:\LocalMachine\' + $storeName + '\' + $thumbprint) } finally { $store.Close() }
    }
    $signature = Get-AuthenticodeSignature -LiteralPath $installed
    if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Thumbprint -ne $thumbprint) { throw '설치된 시험 EXE 서명이 유효하지 않습니다.' }
    [IO.File]::WriteAllText((Join-Path $directory 'operator.ini'), "[trial]`r`nsession=$session`r`noperatorSid=$($identity.User.Value)`r`n", [Text.Encoding]::ASCII)
    @{ schema = 'krasis.system-input-install/v1'; sha256 = $hash; thumbprint = $thumbprint } |
        ConvertTo-Json | Set-Content -LiteralPath (Join-Path $directory 'installation.json') -Encoding UTF8
    New-Service -Name $serviceName -BinaryPathName ('"' + $installed + '" --service') -StartupType Manual -DisplayName 'Krasis 일회성 SYSTEM 입력 시험' | Out-Null
    $createdService = $true
    & $sc sdset $serviceName 'D:P(A;;GA;;;SY)(A;;GA;;;BA)' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw '서비스 접근 권한 설정 실패' }
    Write-Output '설치 완료. 서비스는 아직 실행하지 않았습니다.'
    Write-Output 'B에서 PIN 창을 띄울 준비를 한 뒤 관리자 PowerShell에서 Start-Service KrasisSystemInputTrial 을 한 번 실행하세요.'
    Write-Output '2분 안에 PIN 창을 띄우세요. 시험 결과는 Program Files\KrasisSystemInputTrial\result.json 에 저장됩니다.'
} catch {
    $originalFailure = $_
    if ($createdDirectory) {
        $registered = Get-CimInstance Win32_Service -Filter "Name='KrasisSystemInputTrial'"
        if ($registered) {
            if ($registered.PathName -ne ('"' + $installed + '" --service')) { throw '서비스 소유권이 달라 롤백을 중단했습니다.' }
            if ($registered.State -ne 'Stopped') { Stop-Service -Name $serviceName; (Get-Service -Name $serviceName).WaitForStatus('Stopped', [TimeSpan]::FromSeconds(15)) }
            & $sc delete $serviceName | Out-Null
            if ($LASTEXITCODE -ne 0) { throw '서비스 롤백 실패. 설치 파일을 유지합니다.' }
            $until = (Get-Date).AddSeconds(15)
            while (Get-Service -Name $serviceName -ErrorAction SilentlyContinue) {
                if ((Get-Date) -ge $until) { throw '서비스 삭제 대기 실패. 설치 파일을 유지합니다.' }
                Start-Sleep -Milliseconds 200
            }
        }
        if (Get-CimInstance Win32_Process -Filter "Name='KrasisSystemInputTrial.exe'" | Where-Object {$_.ExecutablePath -eq $installed}) { throw '시험 프로세스가 실행 중이어서 설치 파일을 유지합니다.' }
        foreach ($name in @('KrasisSystemInputTrial.exe', 'operator.ini', 'installation.json', 'run-claimed', 'result.json', 'service-result.json')) {
            $file = Join-Path $directory $name
            if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file -Force }
        }
        Remove-Item -LiteralPath $directory -Force
    }
    foreach ($store in $trustedStores) { if (Test-Path -LiteralPath $store) { Remove-Item -LiteralPath $store -Force } }
    throw $originalFailure
}
