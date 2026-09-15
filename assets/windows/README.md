# Krasis Windows icon

The user selected concept 1 (off-white/violet geometric K on charcoal) on
2026-09-15. `krasis-source.png` is the unchanged built-in ImageGen output,
SHA-256 `2d895aec4e61cfaa42d055f72e4666a88dbb5c0303d402798c562329956c4a8f`.
It is an original AI-generated concept, not a trademark-clearance claim.

`scripts/build-windows-icon.ps1` scales that source without redesign or cropping
to 16, 20, 24, 32, 40, 48, 64, 128 and 256 px PNG-backed ICO frames. It compiles
icon group 1 into checked-in `cmd/*/icon_windows_amd64.syso` resources using the
local LLVM resource tools. Ordinary Go builds and end users need no C compiler
or external image file. Resource regeneration requires Windows System.Drawing
and the local LLVM resource tools; it does not start Krasis or change system icons.

The original PNG, ICO and resource objects are intentionally versioned so clean,
offline Windows amd64 builds keep the selected icon. Other architectures are
not covered by these COFF objects. Use the configured `krasis_icon_assets` intent
for this selected-source import and regeneration. Generated resource timestamps
are fixed to zero.
