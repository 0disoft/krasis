package peerupdate

import "github.com/0disoft/krasis/internal/ziplimit"

func clipboardZIPWithinEntryLimit(data []byte) bool {
	return ziplimit.WithinEntryLimit(data, MaxClipboardFiles)
}
