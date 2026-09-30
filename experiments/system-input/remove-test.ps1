param([Parameter(Mandatory = $true)][string]$Thumbprint)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'B의 관리자 PowerShell에서 실행하세요.' }
if ($Thumbprint -notmatch '^[a-fA-F0-9]{40}$') { throw '정확한 시험 인증서 지문이 필요합니다.' }
$thumbprint = $Thumbprint.ToUpperInvariant()
$name = 'KrasisSystemInputTrial'
$sc = Join-Path ([Environment]::GetFolderPath('System')) 'sc.exe'
$directory = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) $name
$installed = Join-Path $directory ($name + '.exe')
$allowed = @('KrasisSystemInputTrial.exe','installation.json','operator.ini','run-claimed','result.json','service-result.json')
if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw '시험 폴더가 재분석 지점입니다.' }
foreach ($entry in Get-ChildItem -LiteralPath $directory -Force) {
    if ($entry.PSIsContainer -or $entry.Name -notin $allowed -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw '예상하지 못한 파일 또는 재분석 지점이 있습니다.' }
}
$manifestPath = Join-Path $directory 'installation.json'
if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) { throw '시험 설치 기록이 없어 제거를 중단합니다.' }
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
if ($manifest.schema -ne 'krasis.system-input-install/v1' -or $manifest.thumbprint -ne $thumbprint -or
    (Get-FileHash -LiteralPath $installed -Algorithm SHA256).Hash.ToLowerInvariant() -ne $manifest.sha256 -or
    (Get-AuthenticodeSignature -LiteralPath $installed).SignerCertificate.Thumbprint -ne $thumbprint) { throw '시험 설치물의 소유권 또는 해시가 다릅니다.' }
$service = Get-CimInstance Win32_Service -Filter "Name='KrasisSystemInputTrial'"
if ($service) {
    if ($service.PathName -ne ('"' + $installed + '" --service') -or $service.StartName -ne 'LocalSystem') { throw '서비스 실행 경로 또는 계정이 다릅니다.' }
    if ($service.State -ne 'Stopped') { Stop-Service -Name $name; (Get-Service -Name $name).WaitForStatus('Stopped', [TimeSpan]::FromSeconds(15)) }
    & $sc delete $name | Out-Null
    if ($LASTEXITCODE -ne 0) { throw '서비스 제거 실패' }
}
$deadline = (Get-Date).AddSeconds(15)
while (Get-Service -Name $name -ErrorAction SilentlyContinue) {
    if ((Get-Date) -ge $deadline) { throw '서비스 삭제 대기 시간이 초과됐습니다. 파일을 유지합니다.' }
    Start-Sleep -Milliseconds 200
}
$processes = Get-CimInstance Win32_Process -Filter "Name='KrasisSystemInputTrial.exe'" | Where-Object {$_.ExecutablePath -eq $installed}
if ($processes) { throw '시험 프로세스가 아직 실행 중입니다. 파일과 인증서를 유지합니다.' }
foreach ($store in @('Root','TrustedPublisher')) {
    $path = 'Cert:\LocalMachine\' + $store + '\' + $thumbprint
    if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force }
    if (Test-Path -LiteralPath $path) { throw '시험 인증서 제거 확인 실패' }
}
foreach ($file in Get-ChildItem -LiteralPath $directory -File -Force | Where-Object {$_.Name -ne 'installation.json'}) { Remove-Item -LiteralPath $file.FullName -Force }
Remove-Item -LiteralPath $manifestPath -Force
Remove-Item -LiteralPath $directory -Force
Write-Output '시험 서비스·프로세스·설치 파일·해당 시험 인증서를 제거했습니다.'
