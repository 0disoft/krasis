package inputworker

import (
	"bufio"
	"io"
)

// The worker submits each ACK, including diagnostics/cursor suffixes, in one
// Write. Let the first kernel Read receive that whole write instead of forcing
// another overlapped operation after the 24-byte header. Partial reads still
// follow ordinary framing and deadlines. Keep this reader for the entire client
// lifetime: prefetched bytes may never be discarded between exchanges.
const maxResponseSize = responseSize + workerApplyDetailSize + workerSubmissionDetailSize + 26

func newACKReader(reader io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(reader, maxResponseSize)
}
