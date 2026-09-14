//go:build windows

package productidentity

import "testing"

func TestDiagnosticsCanBePreparedAgainWithoutIdentity(t *testing.T) {
	base := t.TempDir()
	for i := 0; i < 2; i++ {
		dir, err := PrepareDiagnostics(base)
		if err != nil {
			t.Fatal(err)
		}
		dir.Close()
	}
}
