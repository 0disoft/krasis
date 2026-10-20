package inputworker

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"

	"github.com/0disoft/krasis/internal/wininput"
)

func TestSourceClientReaderKeepsTerminalCauseInFixedCategories(t *testing.T) {
	for _, test := range []struct {
		err     error
		retired bool
		reason  string
	}{
		{nil, true, "retired"},
		{nil, false, "other"},
		{wininput.ErrEmergencyRelease, true, "emergency"},
		{ErrRecovery, false, "cleanup"},
		{errors.Join(ErrRecovery, io.EOF), true, "cleanup"},
		{io.ErrUnexpectedEOF, false, "truncated"},
		{io.EOF, false, "eof"},
		{context.DeadlineExceeded, false, "timeout"},
		{os.ErrDeadlineExceeded, false, "timeout"},
		{net.ErrClosed, false, "closed"},
		{os.ErrClosed, false, "closed"},
		{context.Canceled, false, "cancelled"},
		{ErrProtocol, false, "protocol"},
		{ErrAuthority, false, "authority"},
		{errors.New("PRIVATE input and native details"), false, "other"},
	} {
		if code := sourceClientReaderCode(test.err, test.retired); code != "installed_source_reader_"+test.reason {
			t.Fatal("terminal cause was erased or leaked", code, test.reason)
		}
		if !test.retired {
			if code := sourceStartupFailureCode(test.err); code != "installed_source_startup_"+test.reason {
				t.Fatal("startup cause was erased or leaked", code, test.reason)
			}
		}
	}
}
