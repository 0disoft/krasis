package inputworker

// Fixed aggregate observations only: no key codes, button IDs or input content.
func sourcePressedEvidence(code string) byte {
	switch code {
	case "boundary_pressed_native_only":
		return 1
	case "boundary_pressed_hook_only":
		return 2
	case "boundary_pressed_native_and_hook":
		return 3
	}
	return 0
}

func sourcePressedCode(evidence byte) string {
	switch evidence {
	case 1:
		return "boundary_pressed_native_only"
	case 2:
		return "boundary_pressed_hook_only"
	case 3:
		return "boundary_pressed_native_and_hook"
	}
	return ""
}
