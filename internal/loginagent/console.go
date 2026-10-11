package loginagent

import "encoding/binary"

// Fixed WTS_INFO_CLASS ABI values. No caller-selected session or class enters
// the service: this reader follows only the active physical console.
const (
	wtsUserName           = 5
	wtsConnectState       = 8
	wtsClientProtocolType = 16
	wtsActive             = 0
	wtsConnected          = 1
)

func readConsole(active func() uint32, query func(uint32, uint32) ([]byte, error)) (Console, error) {
	session := active()
	if session == 0 || session == ^uint32(0) {
		return Console{State: Unavailable}, nil
	}
	state, err := query(session, wtsConnectState)
	if err != nil || len(state) != 4 {
		return Console{}, ErrObservation
	}
	protocol, err := query(session, wtsClientProtocolType)
	if err != nil || len(protocol) != 2 {
		return Console{}, ErrObservation
	}
	name, err := query(session, wtsUserName)
	if err != nil || len(name) < 2 || len(name)%2 != 0 || len(name) > 4096 {
		return Console{}, ErrObservation
	}
	// A complete NUL-terminated UTF-16 value is required. Embedded NULs must
	// not make a malformed nonempty buffer look like a logged-out console.
	if name[len(name)-2] != 0 || name[len(name)-1] != 0 {
		return Console{}, ErrObservation
	}
	for i := 0; i < len(name)-2; i += 2 {
		if binary.LittleEndian.Uint16(name[i:]) == 0 {
			return Console{}, ErrObservation
		}
	}
	if active() != session {
		return Console{}, ErrObservation
	}
	connection := binary.LittleEndian.Uint32(state)
	// WTSActive describes a signed-in active session; a physical console
	// waiting at LogonUI can be WTSConnected. Require both the physical
	// console protocol and an empty user before granting BeforeLogin below.
	if (connection != wtsActive && connection != wtsConnected) || binary.LittleEndian.Uint16(protocol) != 0 {
		return Console{Session: session, State: Unavailable}, nil
	}
	if len(name) > 2 {
		return Console{Session: session, State: UserSession}, nil
	}
	return Console{Session: session, State: BeforeLogin}, nil
}
