$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# Verification-only snapshot, checked against the publisher on 2026-09-14.
# No system installation, registry writes, global PATH changes or product dependency.
# Update the archive and its publisher digest together when adopting a newer toolchain.
$name = 'llvm-mingw-20260908-ucrt-x86_64'
$sha256 = '1bcf74d06b724aeecaa6412ca85f5b26fb1da770e7cdcefa9263c9c5c3ad34b6'
$url = "https://github.com/mstorsjo/llvm-mingw/releases/download/20260908/$name.zip"
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$cache = Join-Path $root '.cache'
$tools = Join-Path $cache 'tools'
$target = Join-Path $tools $name
$archive = Join-Path $tools "$name.zip"
$compiler = Join-Path $target 'bin/x86_64-w64-mingw32-clang.exe'
foreach ($path in @($cache, $tools, $target, $archive)) {
    if ((Test-Path -LiteralPath $path) -and ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw 'Refusing a redirected toolchain cache path.'
    }
}
if (Test-Path -LiteralPath $target) {
    throw 'Toolchain target already exists; inspect it instead of overwriting.'
}
[IO.Directory]::CreateDirectory($tools) | Out-Null
if (-not (Test-Path -LiteralPath $archive)) {
    & curl.exe --fail --location --silent --show-error --max-time 240 --output $archive $url
    if ($LASTEXITCODE -ne 0) { throw 'Toolchain download failed; no archive was executed.' }
}
if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $sha256) {
    throw 'Toolchain archive checksum mismatch; refusing extraction.'
}
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [IO.Compression.ZipFile]::OpenRead($archive)
try {
    $total = 0L
    foreach ($entry in $zip.Entries) {
        $relative = $entry.FullName.Replace('\', '/')
        if (-not $relative.StartsWith("$name/", [StringComparison]::Ordinal) -or $relative.Contains(':') -or ($relative.Split('/') -contains '..')) {
            throw 'Archive entry is outside the expected toolchain directory.'
        }
        $total += $entry.Length
        if ($total -gt 3GB) { throw 'Archive exceeds the extraction size budget.' }
    }
} finally { $zip.Dispose() }
[IO.Compression.ZipFile]::ExtractToDirectory($archive, $tools)
& $compiler --version
if ($LASTEXITCODE -ne 0) { throw 'Compiler probe failed.' }
$library = & $compiler --print-file-name=libsynchronization.a
if ($LASTEXITCODE -ne 0 -or -not [IO.Path]::IsPathRooted($library) -or -not (Test-Path -LiteralPath $library -PathType Leaf)) {
    throw 'Compiler does not provide the synchronization library required by Go race detection.'
}
Write-Output 'Verified portable race toolchain installed in .cache/tools; system PATH unchanged.'
