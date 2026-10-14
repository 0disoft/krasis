Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$snapshot = '69ba48b70d19decbfb4b65fc748567ca5f017bf5'
$baseline = 'e2a1deb7e741f3e6a6e08a910fa9517ad242209f'
$previous = '51796586251d8665ee5cdff26f85b6c498cb2e68'
$peer = '1aeff94249a3a7a76cfa2765948a731e4b57cc898da1ab79f1bd64fa2583e3f0'
$question = [IO.File]::ReadAllText((Join-Path (Get-Location) 'docs/reviews/g293-protected-service-question.md'))
if (-not $question.Contains($snapshot) -or -not $question.Contains($baseline)) { throw 'Question revision mismatch' }
$paths = @(& git -c core.quotepath=false ls-tree -r --name-only $snapshot)
if ($LASTEXITCODE -ne 0 -or $paths.Count -eq 0) { throw 'Missing committed snapshot' }
$excluded = @($paths | Where-Object {
    $_ -match '(^|/)(\.git|\.cache|cache|node_modules|dist|build|target|vendor|secrets|reports|diagnostics|__pycache__)(/|$)' -or
    $_ -match '(?i)(\.(exe|dll|syso|pdb|msi|msix|zip|pem|key|pfx|p12|jsonl|log)$|(^|/)\.env($|\.))'
})
$included = @($paths | Where-Object { $_ -notin $excluded })
if (@($paths | Where-Object { $_ -match '(^/|(^|/)\.\.(/|$)|\\)' }).Count) { throw 'Unsafe source archive path' }
$changed = @(& git diff --name-only $baseline $snapshot)
if ($LASTEXITCODE -ne 0) { throw 'Missing change range' }
$oldPaths = @(& git -c core.quotepath=false ls-tree -r --name-only $baseline)
if ($LASTEXITCODE -ne 0) { throw 'Missing baseline snapshot' }
$baselinePaths = @($changed | Where-Object { $_ -in $oldPaths -and $_ -in $included })
$output = Join-Path (Get-Location) 'dist/reviews/g293-protected-service'
$downloads = Join-Path $env:USERPROFILE 'Downloads'
$archiveName = 'krasis-g293-protected-service-review.zip'
$questionName = 'krasis-g293-QUESTION.md'
foreach ($target in @($output, (Join-Path $downloads $archiveName), (Join-Path $downloads $questionName))) {
    if (Test-Path -LiteralPath $target) { throw 'Review output exists; refusing to overwrite' }
}
New-Item -ItemType Directory -Path $output | Out-Null
$zipPath = Join-Path $output $archiveName
& git archive --format=zip --prefix=source/ "--output=$zipPath" $snapshot
if ($LASTEXITCODE -ne 0) { throw 'Source archive failed' }
$baselineZip = Join-Path $output 'baseline-temporary.zip'
& git archive --format=zip --prefix=source-g291/ "--output=$baselineZip" $baseline -- @baselinePaths
if ($LASTEXITCODE -ne 0) { throw 'Baseline archive failed' }
$patch = (& git diff --no-ext-diff $baseline $snapshot) -join "`n"
if ($LASTEXITCODE -ne 0) { throw 'Change patch failed' }

function Select-Fields($value, [string[]]$names) {
    if ($null -eq $value) { return $null }
    $selected = [ordered]@{}
    foreach ($name in $names) {
        if ($value.PSObject.Properties.Name -contains $name) { $selected[$name] = $value.$name }
    }
    return $selected
}

$extras = [ordered]@{ 'QUESTION.md' = $question; 'changes/g291-to-g293.patch' = $patch + "`n" }
$timeline = [Collections.Generic.List[object]]::new()
$evidence = [Collections.Generic.List[object]]::new()
$cases = @(
    @{ name = 'A-g291'; side = 'A'; request = '9c8cea477956812f766fb4d2e8eaaa8e'; suffix = 'local'; build = $baseline; start = '2026-10-14T10:38:45Z'; end = '2026-10-14T10:43:05Z' },
    @{ name = 'B-g291'; side = 'B'; request = '9c8cea477956812f766fb4d2e8eaaa8e'; suffix = "peer-$peer"; build = $baseline; start = '2026-10-14T10:38:45Z'; end = '2026-10-14T10:43:05Z' },
    @{ name = 'B-g290'; side = 'B'; request = 'e90a4fda275b41c1ba733285092247fc'; suffix = "peer-$peer"; build = $previous; start = '2026-10-14T09:50:00Z'; end = '2026-10-14T09:52:05Z' }
)
foreach ($case in $cases) {
    $file = Join-Path $env:LOCALAPPDATA "Krasis/diagnostics/support-request-$($case.request)-$($case.suffix).json"
    $data = [IO.File]::ReadAllText($file) | ConvertFrom-Json -DateKind String
    if ($data.schema -ne 'krasis.support/v1') { throw 'Unexpected support schema' }
    $logs = @()
    $count = 0
    foreach ($log in $data.logs) {
        $events = @($log.events | Where-Object {
            ($_.PSObject.Properties.Name -contains 'build') -and $_.build -eq $case.build -and
            [DateTimeOffset]::Parse($_.time) -ge [DateTimeOffset]::Parse($case.start) -and
            [DateTimeOffset]::Parse($_.time) -le [DateTimeOffset]::Parse($case.end)
        } | ForEach-Object {
            Select-Fields $_ @('time', 'code', 'build', 'run', 'connection', 'sequence', 'elapsedMicros', 'runtime', 'boundary', 'handoff', 'retry', 'injection')
        })
        if ($events.Count -eq 0) { continue }
        $logs += [ordered]@{ source = $log.source; incomplete = $log.incomplete; events = $events }
        $count += $events.Count
        foreach ($event in $events) { $timeline.Add([ordered]@{ case = $case.name; side = $case.side; source = $log.source; event = $event }) }
    }
    if ($count -eq 0) { throw "Missing incident evidence for $($case.name)" }
    $runtime = Select-Fields $data.loginRuntime @('startedAt', 'time', 'service', 'console', 'grant', 'lastWorker')
    $failure = Select-Fields $data.loginRuntime.lastFailure @('time', 'stage', 'reason', 'operation', 'win32Error', 'win32Known', 'workerExit')
    $runtime['lastFailure'] = $failure
    $record = [ordered]@{
        schema = $data.schema; created = $data.created; scope = $data.scope
        note = 'Historical sanitized receipt and version/time-filtered events; not live health or an independent hardware replay.'
        loginRuntime = $runtime
        installation = Select-Fields $data.installation @('time', 'action', 'result', 'stage', 'phase', 'operation', 'win32Error', 'win32Known', 'recovery', 'recoveryWin32Error', 'recoveryWin32Known', 'closeWin32Error', 'closeWin32Known')
        inputHealth = Select-Fields $data.inputHealth @('time', 'state', 'stage', 'win32Error', 'win32Known')
        logs = $logs
    }
    $extras["evidence/$($case.name).json"] = $record | ConvertTo-Json -Depth 40
    $evidence.Add([ordered]@{
        case = $case.name; request = $case.request; build = $case.build; startUTC = $case.start; endUTC = $case.end
        events = $count; originalSupportSha256 = (Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant()
    })
}
$extras['evidence/timeline.json'] = @($timeline | Sort-Object { [DateTimeOffset]::Parse($_.event.time) }, case) | ConvertTo-Json -Depth 40
$extras['evidence/OBSERVATIONS.md'] = @'
# Observations and validation limits

The g291 incident pair was exported together at 2026-10-14T10:45:52Z. A failed at
10:39:56Z (boundary query/network first, capture service next); B failed at
10:42:05Z (capture/desktop-thread-exit). These are distinct events, not a single
simultaneous failure. The older B-g290 receipt records input/worker-report-read.
Some event logs are incomplete; absence of a code does not prove absence of a path.

Local Windows test before patch:
go test ./internal/inputdesktop -run TestCaptureRetirementWaitsForRealWindowsThreadWithoutInput -count=20 -timeout=45s
Result: FAIL, native thread exit unconfirmed / desktop cleanup failure after about one second.
After patch, the same test at -count=100 passed. It exercises native thread/handle
retirement but never creates Windows input, hooks, windows or protected desktops.
Related Go tests and vet, UI 80 tests and workspace package verification passed.
Full wininput vet has six pre-existing unsafe.Pointer warnings in untouched files.
These recorded outcomes are not a raw replay log and are not hardware PIN success.

As of package preparation, g293 UI/agent update was observed on both PCs. A doctor
reported CONNECTED / PORTABLE, stopped service and installed g291 agent. The B
failure receipt is historical. The new g293 protected service has not been tested
after reboot. No reboot, service restart or new PIN interaction was performed to
prepare this package. All report fields keep their original times and unknowns.

Reported B Application event 1000/1001 check for 18:50-18:52 KST found no
krasis-input.exe entry. This is a human/other-Codex report, not an exported event
log, and cannot exclude child exit or crash. Prior Windows UBR and Deskflow
observations in QUESTION.md are historical reports, not current comparable trials.

No PIN, clipboard contents, screenshot, runtime settings, IP address, certificate,
private key, raw diagnostic stream, build executable, cache or Git history is included.
Tracked icon .syso resources were omitted; their source generation script remains.
'@
$manifest = [ordered]@{
    schema = 'krasis.independent-review-package/v1'; generation = 293; sourceRevision = $snapshot; baselineRevision = $baseline
    sourceTree = (& git rev-parse "$snapshot^{tree}").Trim(); sourceFileCount = $included.Count
    baselineFileCount = $baselinePaths.Count; excludedSourcePaths = $excluded; evidence = @($evidence)
    reviewEntriesSha256 = [ordered]@{}
}
if ($LASTEXITCODE -ne 0) { throw 'Source tree lookup failed' }
foreach ($name in $extras.Keys) {
    $manifest.reviewEntriesSha256[$name] = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($extras[$name]))).ToLowerInvariant()
}
$extras['MANIFEST.json'] = $manifest | ConvertTo-Json -Depth 10
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [IO.Compression.ZipFile]::Open($zipPath, [IO.Compression.ZipArchiveMode]::Update)
$old = [IO.Compression.ZipFile]::OpenRead($baselineZip)
try {
    foreach ($path in $excluded) { $archive.GetEntry("source/$path").Delete() }
    foreach ($entry in $old.Entries) {
        if ($entry.FullName.EndsWith('/')) { continue }
        $destination = $archive.CreateEntry($entry.FullName, [IO.Compression.CompressionLevel]::Optimal)
        $reader = $entry.Open(); $writer = $destination.Open()
        try { $reader.CopyTo($writer) } finally { $reader.Dispose(); $writer.Dispose() }
    }
    foreach ($name in $extras.Keys) {
        $entry = $archive.CreateEntry($name, [IO.Compression.CompressionLevel]::Optimal)
        $writer = [IO.StreamWriter]::new($entry.Open(), [Text.UTF8Encoding]::new($false))
        try { $writer.Write($extras[$name]) } finally { $writer.Dispose() }
    }
} finally { $old.Dispose(); $archive.Dispose() }
Remove-Item -LiteralPath $baselineZip
$archive = [IO.Compression.ZipFile]::OpenRead($zipPath)
try {
    $expected = @($included | ForEach-Object { "source/$_" }) + @($baselinePaths | ForEach-Object { "source-g291/$_" }) + @($extras.Keys)
    $actual = @($archive.Entries | Where-Object { -not $_.FullName.EndsWith('/') } | ForEach-Object FullName)
    if (@(Compare-Object ($expected | Sort-Object) ($actual | Sort-Object)).Count -or $actual.Count -ne @($actual | Select-Object -Unique).Count) { throw 'Archive inventory mismatch' }
    foreach ($entry in $archive.Entries) {
        if ($entry.FullName.EndsWith('/')) { continue }
        $reader = $entry.Open(); $content = [IO.MemoryStream]::new()
        try {
            $reader.CopyTo($content)
            if ($content.Length -ne $entry.Length) { throw 'Truncated archive entry' }
            if ($manifest.reviewEntriesSha256.Contains($entry.FullName)) {
                $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($content.ToArray())).ToLowerInvariant()
                if ($hash -ne $manifest.reviewEntriesSha256[$entry.FullName]) { throw 'Review entry hash mismatch' }
            }
        } finally { $reader.Dispose(); $content.Dispose() }
    }
} finally { $archive.Dispose() }
[IO.File]::WriteAllText((Join-Path $output $questionName), $question, [Text.UTF8Encoding]::new($false))
$hash = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText((Join-Path $output 'SHA256SUMS.txt'), "$hash  $archiveName`n", [Text.UTF8Encoding]::new($false))
Copy-Item -LiteralPath $zipPath -Destination (Join-Path $downloads $archiveName)
Copy-Item -LiteralPath (Join-Path $output $questionName) -Destination (Join-Path $downloads $questionName)
if ((Get-FileHash -LiteralPath (Join-Path $downloads $archiveName) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $hash) { throw 'Downloads archive copy mismatch' }
[pscustomobject]@{ sourceRevision = $snapshot; sourceFiles = $included.Count; baselineFiles = $baselinePaths.Count; evidence = @($evidence); bytes = (Get-Item -LiteralPath $zipPath).Length; sha256 = $hash; verified = $true } | ConvertTo-Json -Depth 6 -Compress
