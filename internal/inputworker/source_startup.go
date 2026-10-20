package inputworker

import (
	"bytes"
	"io"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/wininput"
)

// Native desktop switching can finish a clean source child before it announces
// readiness. Accept only its explicit retirement proof, never an idle event or
// an arbitrary frame as startup success.
func readSourceStartup(reader io.Reader, binding Binding) (*handoff.StartupProfile, bool, error) {
	var prefix [8]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return nil, false, err
	}
	if string(prefix[:4]) != "KSC1" {
		profile, err := readStartupReady(io.MultiReader(bytes.NewReader(prefix[:]), reader))
		return profile, false, err
	}
	p, err := readSourcePacket(io.MultiReader(bytes.NewReader(prefix[:]), reader))
	if err != nil || p.kind != sourceRetired || p.binding != binding || p.sequence != 1 {
		return nil, false, ErrProtocol
	}
	if p.status != 0 {
		// An authenticated failed retirement is valid framing, but never proof
		// of joined native owners. Preserve the same cleanup failure as the
		// live reader instead of disguising it as a protocol violation.
		return nil, false, ErrRecovery
	}
	if p.op == 1 {
		return nil, false, wininput.ErrEmergencyRelease
	}
	return nil, true, nil
}
