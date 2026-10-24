package peerupdate

import "encoding/binary"

// Bound directory parsing before archive/zip allocates a File per entry. The
// advertised count is insufficient: archive/zip compares only its low 16 bits.
func clipboardZIPWithinEntryLimit(data []byte) bool {
	// Match archive/zip's end-record search, including comments and trailing data.
	for _, window := range []int{1024, 65 * 1024} {
		start := max(0, len(data)-window)
		for end := len(data) - 22; end >= start; end-- {
			if binary.LittleEndian.Uint32(data[end:]) != 0x06054b50 {
				continue
			}
			if int(binary.LittleEndian.Uint16(data[end+20:])) > len(data)-end-22 {
				return false
			}
			return clipboardZIPDirectoryWithinLimit(data, end)
		}
		if start == 0 {
			break
		}
	}
	return false
}

func clipboardZIPDirectoryWithinLimit(data []byte, end int) bool {
	count := uint64(binary.LittleEndian.Uint16(data[end+10:]))
	size := uint64(binary.LittleEndian.Uint32(data[end+12:]))
	offset := uint64(binary.LittleEndian.Uint32(data[end+16:]))
	if count == 0xffff || size == 0xffffffff || offset == 0xffffffff {
		// ZIP64 locator immediately precedes the ordinary end record.
		if end < 20 {
			return false
		}
		locator := data[end-20 : end]
		if binary.LittleEndian.Uint32(locator) != 0x07064b50 ||
			binary.LittleEndian.Uint32(locator[4:]) != 0 || binary.LittleEndian.Uint32(locator[16:]) != 1 {
			return false
		}
		at := binary.LittleEndian.Uint64(locator[8:])
		if len(data) < 56 || at > uint64(len(data)-56) {
			return false
		}
		record := data[int(at) : int(at)+56]
		if binary.LittleEndian.Uint32(record) != 0x06064b50 {
			return false
		}
		count = binary.LittleEndian.Uint64(record[32:])
		size = binary.LittleEndian.Uint64(record[40:])
		offset = binary.LittleEndian.Uint64(record[48:])
		end = int(at)
	}
	if count == 0 || count > MaxClipboardFiles || size > uint64(end) || offset > 1<<63-1 {
		return false
	}
	// archive/zip infers a prefix from the directory size, but may fall back to
	// the recorded offset when that points at a valid header. Check both starts
	// so malformed ZIP64 extras cannot make the parsers choose different limits.
	start := end - int(size)
	if !clipboardZIPHeadersWithinLimit(data, start) {
		return false
	}
	if offset < uint64(start) && !clipboardZIPHeadersWithinLimit(data, int(offset)) {
		return false
	}
	return true
}

func clipboardZIPHeadersWithinLimit(data []byte, offset int) bool {
	for count := 0; len(data)-offset >= 46; count++ {
		header := data[offset:]
		if binary.LittleEndian.Uint32(header) != 0x02014b50 {
			break // archive/zip will validate termination and the final count.
		}
		if count == MaxClipboardFiles {
			return false
		}
		size := 46 + int(binary.LittleEndian.Uint16(header[28:])) +
			int(binary.LittleEndian.Uint16(header[30:])) + int(binary.LittleEndian.Uint16(header[32:]))
		if size > len(data)-offset {
			break // Truncated metadata is left to archive/zip to reject.
		}
		offset += size
	}
	return true
}
