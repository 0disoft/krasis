package winlifecycle

import "github.com/0disoft/krasis/internal/agentlifecycle"

const (
	wmQueryEndSession     = 0x0011
	wmEndSession          = 0x0016
	wmClose               = 0x0010
	wmQuit                = 0x0012
	wmDestroy             = 0x0002
	wmDisplayChange       = 0x007e
	wmPowerBroadcast      = 0x0218
	wmWTSSessionChange    = 0x02b1
	wmLifecycleStop       = 0x8003
	pbtAPMSuspend         = 0x0004
	pbtAPMResumeCritical  = 0x0006
	pbtAPMResumeSuspend   = 0x0007
	pbtAPMResumeAutomatic = 0x0012
	wtsConsoleDisconnect  = 0x0002
	wtsRemoteDisconnect   = 0x0004
	wtsSessionLogoff      = 0x0006
	wtsSessionLock        = 0x0007
)

func eventFromMessage(message uint32, wParam uintptr) (agentlifecycle.Event, bool) {
	switch message {
	case wmPowerBroadcast:
		switch wParam {
		case pbtAPMSuspend:
			return agentlifecycle.EventSuspend, true
		case pbtAPMResumeCritical, pbtAPMResumeSuspend, pbtAPMResumeAutomatic:
			return agentlifecycle.EventResume, true
		}
	case wmWTSSessionChange:
		switch wParam {
		case wtsSessionLock:
			return agentlifecycle.EventSessionLock, true
		case wtsConsoleDisconnect, wtsRemoteDisconnect:
			return agentlifecycle.EventSessionDisconnect, true
		case wtsSessionLogoff:
			return agentlifecycle.EventLogoff, true
		}
	case wmQueryEndSession:
		return agentlifecycle.EventShutdown, true
	case wmEndSession:
		if wParam != 0 {
			return agentlifecycle.EventShutdown, true
		}
	case wmDisplayChange:
		return agentlifecycle.EventDisplayChange, true
	}
	return "", false
}
