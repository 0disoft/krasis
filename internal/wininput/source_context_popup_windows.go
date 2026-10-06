//go:build windows

package wininput

var (
	contextGetWindowLong = user32.NewProc("GetWindowLongPtrW")
	contextGetOwner      = user32.NewProc("GetWindow")
	contextGetAncestor   = user32.NewProc("GetAncestor")
	sourceIsContextPopup = isSourceContextPopup
)

// Window structure plus a local right-click gesture narrows dismissal to an
// owned, nonactivating/tool popup. Never cancel an arbitrary captured control,
// the foreground application itself, or an unowned overlay such as a snipper.
func isSourceContextPopup(capture, foreground uintptr) bool {
	if capture == 0 || foreground == 0 || capture == foreground {
		return false
	}
	style, _, _ := contextGetWindowLong.Call(capture, ^uintptr(15))   // GWL_STYLE -16
	exStyle, _, _ := contextGetWindowLong.Call(capture, ^uintptr(19)) // GWL_EXSTYLE -20
	if uint32(style)&(wsPopup|0x40000000) != wsPopup || uint32(exStyle)&(wsExToolWindow|0x08000000) == 0 {
		return false
	}
	owner, _, _ := contextGetOwner.Call(capture, 4) // GW_OWNER
	root, _, _ := contextGetAncestor.Call(owner, 2) // GA_ROOT
	return owner != 0 && root == foreground
}
