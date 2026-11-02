package inputworker

import "encoding/binary"

// Private to the authenticated native pipe. The fixed ACK offset is validated
// before any I/O. Stamp pinned bytes only immediately before WriteFile; never
// after submission, during cancellation, or from an asynchronous callback.
type serviceSubmissionWriter interface {
	writeServiceResponse(data []byte, offset int) (int, error)
}

func validServiceSubmissionOffset(data []byte, offset int) bool {
	return offset >= responseSize && offset <= len(data)-nativeIPCSize && data[offset] == 4
}

func stampServiceSubmission(data []byte, offset int, at uint64) {
	if at == 0 {
		return
	}
	data[offset+1] |= 4
	binary.BigEndian.PutUint64(data[offset+60:offset+68], at)
}
