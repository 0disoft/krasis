package inputworker

import (
	"context"
	"io"

	"github.com/0disoft/krasis/internal/handoff"
)

// The child emits one fixed diagnostic envelope only after independent native
// authentication and desktop entry. The ordinary fresh heartbeat still follows;
// both exchanges share its original budget and neither may inject input.
func newNativePortableClient(ctx context.Context, process portableTransport) (*Client, handoff.NativeStartupProfile, error) {
	run, cancel := context.WithTimeout(ctx, IdleTimeout)
	defer cancel()
	var profile *handoff.StartupProfile
	err := process.ExchangeInput(run, func(_ io.Writer, reader io.Reader) error {
		var err error
		profile, err = readStartupReady(reader)
		return err
	})
	if err != nil {
		return nil, handoff.NativeStartupProfile{}, err
	}
	client, err := newPortableClient(run, process)
	return client, handoff.NativeStartupFrom(profile), err
}
