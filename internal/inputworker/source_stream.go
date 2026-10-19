package inputworker

import (
	"bufio"
	"io"
)

// One advisory batch at most. This is read-ahead of bytes already delivered,
// not a batching timer: an individual ACK is returned without waiting for more.
// The caller owns this reader for one authenticated source stream only; packet
// decoding, binding, consecutive sequence and epoch checks remain per packet.
const sourceReadBufferSize = 16 * sourcePacketSize

func newSourcePacketReader(stream io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(stream, sourceReadBufferSize)
}
