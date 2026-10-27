package control

import (
	"strconv"
	"testing"
)

// Include plan allocation and journal updates, but exclude key-down setup and
// native I/O. All codes fit the same production PressedToken validation.
func BenchmarkPressedStateRecovery(b *testing.B) {
	for _, count := range []int{8, 256, 4096, 32768, 65535} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				b.StopTimer()
				var state PressedState
				for code := 1; code <= count; code++ {
					if _, err := state.RecordPress(PressedToken{Kind: PressedKeyboardKey, Code: uint32(code)}); err != nil {
						b.Fatal(err)
					}
				}
				b.StartTimer()
				for _, token := range state.ReleasePlan() {
					if released, err := state.RecordRelease(token); err != nil || !released {
						b.Fatal("recovery journal update failed", err)
					}
				}
			}
		})
	}
}
