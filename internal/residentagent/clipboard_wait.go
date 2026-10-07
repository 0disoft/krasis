package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
)

func (o *Operations) WaitClipboard(ctx context.Context, revision uint64) localapi.ClipboardWaitResult {
	if o.updates == nil {
		return localapi.ClipboardWaitResult{Code: "unavailable"}
	}
	return localapi.ClipboardWaitResult{Code: "ok", Revision: o.updates.WaitClipboardChange(ctx, revision)}
}
