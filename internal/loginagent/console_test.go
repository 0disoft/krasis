package loginagent

import (
	"errors"
	"testing"
)

func TestConsoleReaderRefusesAmbiguousAndNonConsoleStates(t *testing.T) {
	for _, tc := range []struct {
		name          string
		session       uint32
		connection    []byte
		protocol      []byte
		user          []byte
		queryErr      error
		switchSession bool
		want          ConsoleState
		wantErr       bool
	}{
		{name: "prelogin", session: 1, want: BeforeLogin},
		{name: "locked user", session: 1, user: []byte{'u', 0, 0, 0}, want: UserSession},
		{name: "session zero", want: Unavailable},
		{name: "no console", session: ^uint32(0), want: Unavailable},
		{name: "RDP", session: 1, protocol: []byte{2, 0}, want: Unavailable},
		{name: "disconnected", session: 1, connection: []byte{4, 0, 0, 0}, want: Unavailable},
		{name: "query denied", session: 1, queryErr: errors.New("denied"), wantErr: true},
		{name: "truncated state", session: 1, connection: []byte{0}, wantErr: true},
		{name: "truncated protocol", session: 1, protocol: []byte{0}, wantErr: true},
		{name: "empty user buffer", session: 1, user: []byte{}, wantErr: true},
		{name: "unterminated user", session: 1, user: []byte{'u', 0}, wantErr: true},
		{name: "odd UTF16", session: 1, user: []byte{0, 0, 0}, wantErr: true},
		{name: "embedded terminator", session: 1, user: []byte{0, 0, 'u', 0, 0, 0}, wantErr: true},
		{name: "console switch", session: 1, switchSession: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			got, err := readConsole(func() uint32 {
				reads++
				if tc.switchSession && reads > 1 {
					return 2
				}
				return tc.session
			}, func(_ uint32, class uint32) ([]byte, error) {
				if tc.queryErr != nil {
					return nil, tc.queryErr
				}
				switch class {
				case wtsConnectState:
					if tc.connection != nil {
						return tc.connection, nil
					}
					return []byte{0, 0, 0, 0}, nil
				case wtsClientProtocolType:
					if tc.protocol != nil {
						return tc.protocol, nil
					}
					return []byte{0, 0}, nil
				case wtsUserName:
					if tc.user != nil {
						return tc.user, nil
					}
					return []byte{0, 0}, nil
				}
				t.Fatal("unapproved WTS information class")
				return nil, nil
			})
			if tc.wantErr {
				if !errors.Is(err, ErrObservation) || got != (Console{}) {
					t.Fatalf("ambiguous state accepted: %+v %v", got, err)
				}
			} else if err != nil || got.State != tc.want {
				t.Fatalf("state: %+v %v", got, err)
			}
		})
	}
}
