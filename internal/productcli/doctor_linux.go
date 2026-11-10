//go:build linux

package productcli

import "context"

func inspectDoctorHost(context.Context, string, string) doctorHost {
	// Linux currently runs a per-user CLI agent. Do not report a Windows UI
	// launch record or SYSTEM input/recovery service as missing or healthy.
	return doctorHost{
		UI: doctorComponent{State: "UNSUPPORTED"}, LaunchRecord: doctorComponent{State: "UNSUPPORTED"},
		InstalledAgent: doctorComponent{State: "UNSUPPORTED"}, Service: doctorComponent{State: "UNSUPPORTED"},
		AgentLocation: "USER_SESSION",
	}
}
