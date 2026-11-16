package inputworker

import (
	"fmt"
	"testing"
)

func TestReplyPublicationYieldsOnlyAfterAcceptedActivation(t *testing.T) {
	for _, op := range []byte{sourceActivate, sourceHeartbeat, sourceAdmit, sourceDeactivate, sourceStop} {
		t.Run(fmt.Sprint(op), func(t *testing.T) {
			replies := make(chan sourcePacket, 1)
			packet := sourcePacket{kind: sourceReply, op: op, sequence: 17, reply: 9}
			yields := 0
			yield := func() {
				yields++
				if len(replies) != 1 {
					t.Fatal("yield preceded publication")
				}
			}
			if !publishSourceReply(replies, packet, yield) {
				t.Fatal("empty reply slot rejected")
			}
			want := 0
			if op == sourceActivate {
				want = 1
			}
			if yields != want {
				t.Fatalf("yields=%d, want %d", yields, want)
			}
			if publishSourceReply(replies, sourcePacket{op: sourceActivate}, yield) || yields != want {
				t.Fatal("full queue accepted or yielded")
			}
			if got := <-replies; got != packet {
				t.Fatal("publication replaced/reordered an existing reply")
			}
		})
	}
	if publishSourceReply(nil, sourcePacket{op: sourceActivate}, func() { t.Fatal("nil queue yielded") }) {
		t.Fatal("nil queue accepted")
	}
}
