package peerupdate

import (
	"bytes"
	"encoding/binary"
	"image/png"
)

const MaxClipboardImageBytes = 40 << 20

// ValidClipboardImage accepts only bounded, ordinary PNG or uncompressed DIB
// pixels. It is checked before either forwarding or installing peer content.
func ValidClipboardImage(format string, data []byte) bool {
	if len(data) == 0 || len(data) > MaxClipboardImageBytes {
		return false
	}
	switch format {
	case "png":
		config, err := png.DecodeConfig(bytes.NewReader(data))
		return err == nil && config.Width > 0 && config.Height > 0 &&
			int64(config.Width)*int64(config.Height) <= 10_000_000
	case "dib":
		if len(data) < 40 {
			return false
		}
		header := binary.LittleEndian.Uint32(data[:4])
		if header != 40 && header != 108 && header != 124 || int(header) > len(data) {
			return false
		}
		width := int32(binary.LittleEndian.Uint32(data[4:8]))
		height := int32(binary.LittleEndian.Uint32(data[8:12]))
		planes := binary.LittleEndian.Uint16(data[12:14])
		bits := binary.LittleEndian.Uint16(data[14:16])
		compression := binary.LittleEndian.Uint32(data[16:20])
		if width <= 0 || height == 0 || height == -1<<31 || planes != 1 ||
			(bits != 24 && bits != 32) || (compression != 0 && compression != 3) {
			return false
		}
		rows := int64(height)
		if rows < 0 {
			rows = -rows
		}
		if int64(width)*rows > 10_000_000 {
			return false
		}
		stride := ((int64(width)*int64(bits) + 31) / 32) * 4
		offset := int64(header)
		if header == 40 && compression == 3 {
			offset += 12 // RGB masks follow BITMAPINFOHEADER.
		}
		return offset+stride*rows <= int64(len(data))
	default:
		return false
	}
}
