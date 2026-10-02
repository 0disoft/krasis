//go:build !windows

package productcli

import "context"

func inspectDoctorHost(context.Context, string, string) doctorHost {
	return doctorHost{UI: doctorComponent{State: "UNKNOWN"}, LaunchRecord: doctorComponent{State: "UNKNOWN"}, InstalledAgent: doctorComponent{State: "UNKNOWN"}, Service: doctorComponent{State: "UNKNOWN"}, AgentLocation: "UNKNOWN"}
}
