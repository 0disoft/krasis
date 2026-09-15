//go:build windows

package launcherui

var (
	callLoadAppIcon    = user32.NewProc("LoadImageW").Call
	callDestroyAppIcon = user32.NewProc("DestroyIcon").Call
	callIconMetric     = user32.NewProc("GetSystemMetrics").Call
)

type appIcon struct {
	handle uintptr
	owned  bool
}

type windowIcons struct{ large, small appIcon }

func loadAppIcon(instance, width, height uintptr) appIcon {
	// Load owned images: LR_SHARED can return the first cached size instead of
	// the requested small/large variant. Resource 1 is generated from krasis.ico.
	handle, _, _ := callLoadAppIcon(instance, 1, 1, width, height, 0)
	if handle != 0 {
		return appIcon{handle: handle, owned: true}
	}
	// Tests and builds without the resource still have a usable shared fallback.
	handle, _, _ = callLoadAppIcon(0, 32512, 1, width, height, 0x8000)
	return appIcon{handle: handle}
}

func loadWindowIcons(instance uintptr) windowIcons {
	w, _, _ := callIconMetric(11)
	h, _, _ := callIconMetric(12)
	sw, _, _ := callIconMetric(49)
	sh, _, _ := callIconMetric(50)
	return windowIcons{large: loadAppIcon(instance, w, h), small: loadAppIcon(instance, sw, sh)}
}

func (icon *appIcon) close() {
	if icon.owned && icon.handle != 0 {
		callDestroyAppIcon(icon.handle)
	}
	icon.handle = 0
	icon.owned = false
}

func (icons *windowIcons) close() { icons.small.close(); icons.large.close() }
