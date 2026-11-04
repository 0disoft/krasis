//go:build windows

package inputworker

import (
	"net"
	"testing"
)

type activationWriteCounter struct {
	net.Conn
	writes int
	bytes  int
}

func (c *activationWriteCounter) Write(p []byte) (int, error) {
	c.writes++
	c.bytes += len(p)
	return c.Conn.Write(p)
}

func TestSourceActivationIPCWritesSuffixWithSingleAuthenticatedACK(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	stream := &activationWriteCounter{Conn: conn}
	w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	w.channel.authority.checkPresent = func() error { return nil }
	request := sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 1, binding: w.binding}
	response := sourcePacket{kind: sourceReply, op: sourceActivate, reply: 1, captureIPC: &sourceActivationTrace{clocks: sourceActivationClocks{1, 2, 3, 4}}}
	got := make(chan sourcePacket, 1)
	go func() { p, _ := readSourcePacket(server); got <- p }()
	if err := w.sendCompletedReply(request, response); err != nil {
		t.Fatal(err)
	}
	p := <-got
	if stream.writes != 1 || stream.bytes != sourcePacketSize+sourceActivationIPCSize || p.reply != 1 || (p.captureIPC == nil || !p.captureIPC.clocks.usable()) {
		t.Fatal("ACK replayed or diagnostic adds I/O", stream.writes, stream.bytes, p)
	}
}
