// Package ziplimit bounds ZIP directory parsing before archive/zip allocates
// per-entry metadata. The source must remain unchanged during construction.
package ziplimit

import (
	"archive/zip"
	"encoding/binary"
	"io"
)

// NewReader inspects bounded metadata without loading the archive's payload.
func NewReader(source io.ReaderAt, size int64, maxEntries int) (*zip.Reader, error) {
	if source == nil || size < 22 || maxEntries < 1 {
		return nil, zip.ErrFormat
	}
	var scratch [65 * 1024]byte
	view := func(offset int64, length int) []byte {
		if offset < 0 || offset > size || int64(length) > size-offset || length > len(scratch) {
			return nil
		}
		data := scratch[:length]
		if n, err := source.ReadAt(data, offset); n != length || err != nil && err != io.EOF {
			return nil
		}
		return data
	}
	if !withinEntryLimit(size, maxEntries, view) {
		return nil, zip.ErrFormat
	}
	reader, err := zip.NewReader(source, size)
	if err != nil {
		return nil, err
	}
	if len(reader.File) == 0 || len(reader.File) > maxEntries {
		return nil, zip.ErrFormat
	}
	return reader, nil
}

// WithinEntryLimit performs the same preflight on existing bytes without
// allocating a metadata buffer. Full ZIP validation must still follow it.
func WithinEntryLimit(data []byte, maxEntries int) bool {
	return withinEntryLimit(int64(len(data)), maxEntries, func(offset int64, length int) []byte {
		if offset < 0 || offset > int64(len(data)) || int64(length) > int64(len(data))-offset {
			return nil
		}
		return data[int(offset) : int(offset)+length]
	})
}

func withinEntryLimit(size int64, maxEntries int, view func(int64, int) []byte) bool {
	if size < 22 || maxEntries < 1 {
		return false
	}
	// Match archive/zip's end-record search, including comments and trailing data.
	for _, window := range []int64{1024, 65 * 1024} {
		start := max(int64(0), size-window)
		data := view(start, int(size-start))
		if data == nil {
			return false
		}
		for end := len(data) - 22; end >= 0; end-- {
			if binary.LittleEndian.Uint32(data[end:]) != 0x06054b50 {
				continue
			}
			if int(binary.LittleEndian.Uint16(data[end+20:])) > len(data)-end-22 {
				return false
			}
			return directoryWithinLimit(size, start+int64(end), maxEntries, view)
		}
		if start == 0 {
			break
		}
	}
	return false
}

func directoryWithinLimit(size, end int64, maxEntries int, view func(int64, int) []byte) bool {
	footer := view(end, 22)
	if footer == nil {
		return false
	}
	count := uint64(binary.LittleEndian.Uint16(footer[10:]))
	directorySize := uint64(binary.LittleEndian.Uint32(footer[12:]))
	offset := uint64(binary.LittleEndian.Uint32(footer[16:]))
	if count == 0xffff || directorySize == 0xffffffff || offset == 0xffffffff {
		locator := view(end-20, 20)
		if locator == nil || binary.LittleEndian.Uint32(locator) != 0x07064b50 ||
			binary.LittleEndian.Uint32(locator[4:]) != 0 || binary.LittleEndian.Uint32(locator[16:]) != 1 {
			return false
		}
		at := binary.LittleEndian.Uint64(locator[8:])
		if size < 56 || at > uint64(size-56) {
			return false
		}
		record := view(int64(at), 56)
		if record == nil || binary.LittleEndian.Uint32(record) != 0x06064b50 {
			return false
		}
		count = binary.LittleEndian.Uint64(record[32:])
		directorySize = binary.LittleEndian.Uint64(record[40:])
		offset = binary.LittleEndian.Uint64(record[48:])
		end = int64(at)
	}
	if count == 0 || count > uint64(maxEntries) || directorySize > uint64(end) || offset > 1<<63-1 {
		return false
	}
	// archive/zip can infer a prefix or fall back to the recorded offset. Bound
	// both candidates; malformed extras must not make the parsers disagree.
	start := end - int64(directorySize)
	if !headersWithinLimit(size, start, maxEntries, view) {
		return false
	}
	return offset >= uint64(start) || headersWithinLimit(size, int64(offset), maxEntries, view)
}

func headersWithinLimit(size, offset int64, maxEntries int, view func(int64, int) []byte) bool {
	for count := 0; size-offset >= 46; count++ {
		header := view(offset, 46)
		if header == nil {
			return false
		}
		if binary.LittleEndian.Uint32(header) != 0x02014b50 {
			break
		}
		if count == maxEntries {
			return false
		}
		length := int64(46) + int64(binary.LittleEndian.Uint16(header[28:])) +
			int64(binary.LittleEndian.Uint16(header[30:])) + int64(binary.LittleEndian.Uint16(header[32:]))
		if length > size-offset {
			break // archive/zip validates truncation and the final count.
		}
		offset += length
	}
	return true
}
