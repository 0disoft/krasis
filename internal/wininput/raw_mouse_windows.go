//go:build windows

package wininput

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	wmInput                    = 0x00FF
	ridInput                   = 0x10000003
	rimTypeMouse               = 0
	ridevRemove                = 0x00000001
	ridevInputSink             = 0x00000100
	hidUsagePageGeneric        = 0x0001
	hidUsageMouse              = 0x0002
	mouseMoveAbsolute          = 0x0001
	rawMouseWheel              = 0x0400
	rawMouseHWheel             = 0x0800
	rawInputBlockAlignment     = 8
	defaultRawInputBatchBytes  = 64 * 1024
	maximumRawInputPacketBytes = 1024 * 1024
)

var (
	procCreateWindowExW         = user32.NewProc("CreateWindowExW")
	procDestroyWindow           = user32.NewProc("DestroyWindow")
	procRegisterRawInputDevices = user32.NewProc("RegisterRawInputDevices")
	procGetRawInputData         = user32.NewProc("GetRawInputData")
	procGetRawInputBuffer       = user32.NewProc("GetRawInputBuffer")
)

type rawInputDevice struct {
	UsagePage uint16
	Usage     uint16
	Flags     uint32
	Target    uintptr
}

type rawInputHeader struct {
	Type   uint32
	Size   uint32
	Device uintptr
	WParam uintptr
}

type rawMouse struct {
	Flags            uint16
	Padding          uint16
	Buttons          uint32
	RawButtons       uint32
	LastX            int32
	LastY            int32
	ExtraInformation uint32
}

type alignedRawInputBuffer struct {
	storage []byte
	data    []byte
}

func newAlignedRawInputBuffer(size int) (alignedRawInputBuffer, error) {
	if size <= 0 || size > maximumRawInputPacketBytes {
		return alignedRawInputBuffer{}, fmt.Errorf("raw input buffer size must be between 1 and %d", maximumRawInputPacketBytes)
	}
	storage := make([]byte, size+rawInputBlockAlignment-1)
	base := uintptr(unsafe.Pointer(&storage[0]))
	offset := int((rawInputBlockAlignment - base%rawInputBlockAlignment) % rawInputBlockAlignment)
	return alignedRawInputBuffer{storage: storage, data: storage[offset : offset+size]}, nil
}

func (buffer alignedRawInputBuffer) pointer() uintptr {
	if len(buffer.data) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&buffer.data[0]))
}

type rawMouseReader struct {
	current alignedRawInputBuffer
	batch   alignedRawInputBuffer
}

func newRawMouseReader() (*rawMouseReader, error) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return nil, fmt.Errorf("raw input reader requires 64-bit Windows layout")
	}
	minimum := int(unsafe.Sizeof(rawInputHeader{})) + int(unsafe.Sizeof(rawMouse{}))
	current, err := newAlignedRawInputBuffer(minimum)
	if err != nil {
		return nil, err
	}
	batch, err := newAlignedRawInputBuffer(defaultRawInputBatchBytes)
	if err != nil {
		return nil, err
	}
	return &rawMouseReader{current: current, batch: batch}, nil
}

func createRawInputMessageWindow(name string) (uintptr, error) {
	className, err := syscall.UTF16PtrFromString("STATIC")
	if err != nil {
		return 0, err
	}
	windowName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	window, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0,
		0, 0, 0, 0,
		^uintptr(2),
		0, 0, 0,
	)
	if window == 0 {
		return 0, fmt.Errorf("CreateWindowExW(raw input) failed: %w", callErr)
	}
	return window, nil
}

func registerRawMouseInput(window uintptr) error {
	if window == 0 {
		return fmt.Errorf("raw input target window is required")
	}
	device := rawInputDevice{
		UsagePage: hidUsagePageGeneric,
		Usage:     hidUsageMouse,
		Flags:     ridevInputSink,
		Target:    window,
	}
	result, _, callErr := procRegisterRawInputDevices.Call(
		uintptr(unsafe.Pointer(&device)),
		1,
		unsafe.Sizeof(device),
	)
	if result == 0 {
		return fmt.Errorf("RegisterRawInputDevices(mouse) failed: %w", callErr)
	}
	return nil
}

func unregisterRawMouseInput() error {
	device := rawInputDevice{UsagePage: hidUsagePageGeneric, Usage: hidUsageMouse, Flags: ridevRemove}
	result, _, callErr := procRegisterRawInputDevices.Call(
		uintptr(unsafe.Pointer(&device)),
		1,
		unsafe.Sizeof(device),
	)
	if result == 0 {
		return fmt.Errorf("RegisterRawInputDevices(mouse remove) failed: %w", callErr)
	}
	return nil
}

func (reader *rawMouseReader) readCurrent(handle uintptr, visit func(rawInputHeader, rawMouse) error) error {
	if reader == nil || len(reader.current.data) == 0 {
		return fmt.Errorf("raw input reader is not initialized")
	}
	if handle == 0 {
		return fmt.Errorf("raw input handle is required")
	}
	if visit == nil {
		return fmt.Errorf("raw input visitor is required")
	}
	headerSize := uint32(unsafe.Sizeof(rawInputHeader{}))
	var required uint32
	result, _, callErr := procGetRawInputData.Call(
		handle,
		ridInput,
		0,
		uintptr(unsafe.Pointer(&required)),
		uintptr(headerSize),
	)
	if uint32(result) == ^uint32(0) {
		return fmt.Errorf("GetRawInputData(size) failed: %w", callErr)
	}
	if required > maximumRawInputPacketBytes {
		return fmt.Errorf("raw input packet exceeds %d bytes: %d", maximumRawInputPacketBytes, required)
	}
	if int(required) > len(reader.current.data) {
		buffer, err := newAlignedRawInputBuffer(int(required))
		if err != nil {
			return err
		}
		reader.current = buffer
	}
	provided := uint32(len(reader.current.data))
	result, _, callErr = procGetRawInputData.Call(
		handle,
		ridInput,
		reader.current.pointer(),
		uintptr(unsafe.Pointer(&provided)),
		uintptr(headerSize),
	)
	if uint32(result) == ^uint32(0) {
		return fmt.Errorf("GetRawInputData(packet) failed: %w", callErr)
	}
	read := uint32(result)
	if read != required || provided != required {
		return fmt.Errorf("GetRawInputData size mismatch: required=%d provided=%d read=%d", required, provided, read)
	}
	return visitRawInputBlocks(reader.current.data[:read], 1, visit)
}

func (reader *rawMouseReader) drainBuffered(visit func(rawInputHeader, rawMouse) error) error {
	if reader == nil || len(reader.batch.data) == 0 {
		return fmt.Errorf("raw input reader is not initialized")
	}
	if visit == nil {
		return fmt.Errorf("raw input visitor is required")
	}
	headerSize := uint32(unsafe.Sizeof(rawInputHeader{}))
	for {
		provided := uint32(len(reader.batch.data))
		result, _, callErr := procGetRawInputBuffer.Call(
			reader.batch.pointer(),
			uintptr(unsafe.Pointer(&provided)),
			uintptr(headerSize),
		)
		if uint32(result) == ^uint32(0) {
			return fmt.Errorf("GetRawInputBuffer failed: %w", callErr)
		}
		count := uint32(result)
		if count == 0 {
			return nil
		}
		if err := visitRawInputBlocks(reader.batch.data, count, visit); err != nil {
			return err
		}
	}
}

func visitRawInputBlocks(buffer []byte, count uint32, visit func(rawInputHeader, rawMouse) error) error {
	if count == 0 {
		return nil
	}
	if visit == nil {
		return fmt.Errorf("raw input visitor is required")
	}
	headerSize := uintptr(unsafe.Sizeof(rawInputHeader{}))
	mouseSize := uintptr(unsafe.Sizeof(rawMouse{}))
	offset := uintptr(0)
	for index := uint32(0); index < count; index++ {
		if offset > uintptr(len(buffer)) || uintptr(len(buffer))-offset < headerSize {
			return fmt.Errorf("raw input block %d has a truncated header", index)
		}
		pointer := unsafe.Pointer(&buffer[offset])
		header := *(*rawInputHeader)(pointer)
		if uintptr(header.Size) < headerSize {
			return fmt.Errorf("raw input block %d has invalid size %d", index, header.Size)
		}
		if uintptr(header.Size) > uintptr(len(buffer))-offset {
			return fmt.Errorf("raw input block %d exceeds the read buffer", index)
		}
		if header.Type == rimTypeMouse {
			minimum := headerSize + mouseSize
			if uintptr(header.Size) < minimum {
				return fmt.Errorf("raw mouse block %d is undersized: %d", index, header.Size)
			}
			mouse := *(*rawMouse)(unsafe.Add(pointer, headerSize))
			if err := visit(header, mouse); err != nil {
				return err
			}
		}
		offset += alignRawInputBlock(uintptr(header.Size))
	}
	return nil
}

func alignRawInputBlock(size uintptr) uintptr {
	return (size + rawInputBlockAlignment - 1) &^ (rawInputBlockAlignment - 1)
}
