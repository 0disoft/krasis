$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$root = Split-Path $PSScriptRoot -Parent
$assets = Join-Path $root 'assets/windows'
$source = Join-Path $assets 'krasis-source.png'
$expected = '2D895AEC4E61CFAA42D055F72E4666A88DBB5C0303D402798C562329956C4A8F'
if ((Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash -ne $expected) {
    throw 'Icon source differs from the user-selected concept.'
}
$image = [Drawing.Image]::FromFile($source)
$frames = [Collections.Generic.List[byte[]]]::new()
$sizes = @(16, 20, 24, 32, 40, 48, 64, 128, 256)
try {
    foreach ($size in $sizes) {
        $bitmap = [Drawing.Bitmap]::new($size, $size, [Drawing.Imaging.PixelFormat]::Format32bppArgb)
        $graphics = [Drawing.Graphics]::FromImage($bitmap)
        $stream = [IO.MemoryStream]::new()
        try {
            $graphics.CompositingMode = [Drawing.Drawing2D.CompositingMode]::SourceCopy
            $graphics.InterpolationMode = [Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
            $graphics.PixelOffsetMode = [Drawing.Drawing2D.PixelOffsetMode]::HighQuality
            $graphics.DrawImage($image, [Drawing.Rectangle]::new(0, 0, $size, $size))
            $bitmap.Save($stream, [Drawing.Imaging.ImageFormat]::Png)
            $frames.Add($stream.ToArray())
        } finally { $stream.Dispose(); $graphics.Dispose(); $bitmap.Dispose() }
    }
} finally { $image.Dispose() }
$output = [IO.MemoryStream]::new()
$writer = [IO.BinaryWriter]::new($output)
try {
    $writer.Write([uint16]0); $writer.Write([uint16]1); $writer.Write([uint16]$sizes.Count)
    $offset = 6 + 16 * $sizes.Count
    for ($i = 0; $i -lt $sizes.Count; $i++) {
        $dimension = $sizes[$i] % 256
        $writer.Write([byte]$dimension); $writer.Write([byte]$dimension)
        $writer.Write([byte]0); $writer.Write([byte]0)
        $writer.Write([uint16]1); $writer.Write([uint16]32)
        $writer.Write([uint32]$frames[$i].Length); $writer.Write([uint32]$offset)
        $offset += $frames[$i].Length
    }
    foreach ($frame in $frames) { $writer.Write($frame) }
    $writer.Flush()
    [IO.File]::WriteAllBytes((Join-Path $assets 'krasis.ico'), $output.ToArray())
} finally { $writer.Dispose(); $output.Dispose() }
$bin = Join-Path $root '.cache/tools/llvm-mingw-20260908-ucrt-x86_64/bin'
$scratch = Join-Path $root '.cache/icon-build'
[IO.Directory]::CreateDirectory($scratch) | Out-Null
$res = Join-Path $scratch 'krasis.res'
$obj = Join-Path $scratch 'krasis.syso'
Push-Location $assets
try {
    & (Join-Path $bin 'llvm-rc.exe') /FO $res 'krasis.rc'
    if ($LASTEXITCODE -ne 0) { throw 'Icon resource compilation failed.' }
    & (Join-Path $bin 'llvm-cvtres.exe') /MACHINE:X64 /TIMESTAMP:0 "/OUT:$obj" $res
    if ($LASTEXITCODE -ne 0) { throw 'Icon COFF conversion failed.' }
} finally { Pop-Location }
foreach ($name in @('krasis', 'krasis-agent', 'krasis-control')) {
    Copy-Item -LiteralPath $obj -Destination (Join-Path $root "cmd/$name/icon_windows_amd64.syso")
}
Write-Output 'Generated 9 PNG-backed ICO sizes and Windows amd64 resources for all three executables.'
