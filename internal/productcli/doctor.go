package productcli

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productlog"
)

type doctorComponent struct {
	State      string `json:"state"`
	Generation int    `json:"generation,omitempty"`
	Build      string `json:"build,omitempty"`
	PathStatus string `json:"pathStatus,omitempty"`
}
type doctorHost struct {
	UI, LaunchRecord, InstalledAgent, Service doctorComponent
	AgentLocation                             string
}
type doctorCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
}
type doctorAction struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// A read-only projection, not input authorization or repair. v1 fields remain.
func runDoctor(ctx context.Context, parsed options, deps dependencies) (doctorResult, int) {
	local := localVersion()
	v := doctorResult{SchemaVersion: doctorSchema, Agent: "UNAVAILABLE", Connection: "UNKNOWN", Protocol: "UNKNOWN", Code: "agent_unavailable", CLIGeneration: local.CLIGeneration, CLIBuild: local.CLIBuild,
		UI: doctorComponent{State: "NOT_CHECKED"}, LaunchRecord: doctorComponent{State: "NOT_CHECKED"}, InstalledAgent: doctorComponent{State: "NOT_CHECKED"}, Service: doctorComponent{State: "NOT_CHECKED"}, AgentLocation: "UNKNOWN", ProtectedInput: "UNKNOWN", Checks: []doctorCheck{}, NextActions: []doctorAction{}}
	exit := ExitAgentUnavailable
	control, err := ResolveControlDirectory(deps.localAppData, parsed.controlDir)
	if err != nil {
		v.Code, exit = "config_invalid", ExitUsage
	} else {
		client, openErr := deps.openClient(control)
		if openErr != nil || client == nil {
			err = openErr
		} else {
			remote, remoteExit, remoteErr := doctor(ctx, client)
			if remoteErr != nil {
				err = remoteErr
			} else {
				v.Agent, v.Connection, v.PeerAlias = remote.Agent, remote.Connection, remote.PeerAlias
				v.AgentGeneration, v.Protocol, v.AgentBuild = remote.AgentGeneration, remote.Protocol, remote.AgentBuild
				if !validHex(v.AgentBuild, 40) && v.AgentBuild != "development" {
					v.AgentBuild = ""
				}
				exit, v.Code = remoteExit, "healthy"
				if remoteExit != ExitOK {
					v.Code = "connection_not_ready"
				}
				if remote.Protocol == "INCOMPATIBLE" {
					v.Code = "protocol_mismatch"
				}
			}
		}
		if err != nil {
			switch {
			case errors.Is(err, localapi.ErrAuthentication):
				v.Agent, v.Code, exit = "UNKNOWN", "local_api_authentication_failed", ExitAuthentication
			case errors.Is(err, localapi.ErrProtocol):
				v.Agent, v.Code, exit = "UNKNOWN", "local_api_protocol_failed", ExitTransport
			case errors.Is(err, localapi.ErrTransport):
				v.Agent, v.Code, exit = "UNKNOWN", "local_api_transport_failed", ExitTransport
			default:
				v.Agent, v.Code, exit = "UNKNOWN", "agent_unavailable", ExitAgentUnavailable
			}
		}
	}
	if deps.inspectHost != nil {
		host := deps.inspectHost(ctx, deps.localAppData, v.AgentBuild)
		v.UI, v.LaunchRecord, v.InstalledAgent, v.Service, v.AgentLocation = host.UI, host.LaunchRecord, host.InstalledAgent, host.Service, host.AgentLocation
	}
	v.addCheck("agent", v.Agent)
	v.addCheck("connection", v.Connection)
	v.addCheck("protocol", v.Protocol)
	if v.Agent == "RUNNING" {
		v.checkMatch("cli_agent_version", v.AgentGeneration == v.CLIGeneration && compatibleBuild(v.CLIBuild, v.AgentBuild), "restart_matching_agent", "CLI와 같은 버전의 앱으로 에이전트를 안전하게 다시 실행하세요.", &exit)
		if v.UI.State == "RUNNING" {
			v.checkMatch("ui_agent_version", v.UI.Generation == v.AgentGeneration && compatibleBuild(v.UI.Build, v.AgentBuild), "restart_matching_ui", "실행 에이전트와 같은 버전의 앱을 여세요.", &exit)
		}
		if v.InstalledAgent.State == "VERIFIED" {
			v.checkMatch("installed_agent_version", compatibleBuild(v.InstalledAgent.Build, v.AgentBuild), "update_input_installation", "앱의 보호 입력 갱신 버튼으로 설치를 맞추세요. Windows 관리자 확인 후 자동으로 진행합니다.", &exit)
		}
	}
	v.addCheck("ui", v.UI.State)
	v.addCheck("launch_record", v.LaunchRecord.State)
	v.addCheck("installed_agent", v.InstalledAgent.State)
	v.addCheck("service", v.Service.State)
	if cleanup, err := productlog.CleanupReceipt(deps.localAppData); err != nil {
		v.addCheck("update_stage_cleanup", "UNKNOWN")
		v.action("inspect_update_cleanup", "업데이트 폴더 정리 결과를 읽지 못했습니다. 진단을 확인하세요.")
	} else if cleanup != nil {
		if cleanup.Code == "failed" {
			v.addCheck("update_stage_cleanup", "FAILED")
			v.action("inspect_update_cleanup", "일부 업데이트 폴더 정리를 보류하거나 실패했습니다. 파일을 수동 삭제하기 전에 진단을 확인하세요.")
		} else {
			v.addCheck("update_stage_cleanup", "COMPLETED")
		}
	}
	if v.UI.State == "NOT_RUNNING" && v.LaunchRecord.State == "VERIFIED" {
		v.action("open_ui", "앱 창이 닫혀 있습니다. krasis-cli relaunch로 열 수 있습니다.")
	}
	if v.UI.State == "RECORD_INVALID" || v.UI.State == "RUNNING_UNVERIFIED" {
		v.problem("launch_record_invalid", "repair_launch_record", "확인한 최신 배포 폴더에서 krasis-cli app repair로 복구 경로를 갱신하세요.", &exit)
	}
	if v.LaunchRecord.State == "INVALID" || v.LaunchRecord.State == "NOT_FOUND" {
		v.problem("launch_record_invalid", "repair_launch_record", "확인한 최신 배포 폴더에서 krasis-cli app repair로 복구 경로를 갱신하세요.", &exit)
	}
	if v.Service.PathStatus == "MISMATCH" {
		v.problem("service_path_mismatch", "inspect_service_installation", "서비스 실행 설정이 고정 설치 위치와 다릅니다. 설치 상태를 확인하세요.", &exit)
	}
	if v.InstalledAgent.State == "INVALID" {
		v.problem("input_installation_invalid", "inspect_input_installation", "설치 파일 검증에 실패했습니다. 진단을 내보내고 설치 상태를 확인하세요.", &exit)
	}
	if v.InstalledAgent.State == "VERIFIED" && v.Service.State == "STOPPED" {
		v.problem("input_service_stopped", "inspect_input_service", "입력 서비스가 중지돼 있습니다. 서비스 진단을 확인하세요.", &exit)
	}
	if v.InstalledAgent.State == "VERIFIED" && (v.Service.State == "NOT_INSTALLED" || v.Service.State == "PENDING") {
		v.problem("input_service_not_ready", "inspect_input_service", "설치된 입력 서비스가 아직 준비되지 않았습니다. 서비스 진단과 설치 결과를 확인하세요.", &exit)
	}
	if v.AgentLocation == "PROTECTED" && v.Service.State == "RUNNING" && v.Service.PathStatus == "MATCHED" && v.InstalledAgent.State == "VERIFIED" && compatibleBuild(v.AgentBuild, v.InstalledAgent.Build) {
		v.ProtectedInput = "CONFIGURED"
	} else if v.AgentLocation == "PORTABLE" {
		v.ProtectedInput = "PORTABLE"
		v.action("portable_input", "일반 입력 경로입니다. 관리자 창 제어에는 같은 버전의 보호된 입력 설치와 실행 경로를 확인하세요.")
	}
	if deps.inspectHost != nil && (v.UI.State == "UNKNOWN" || v.LaunchRecord.State == "UNKNOWN" || v.InstalledAgent.State == "UNKNOWN" || v.Service.State == "UNKNOWN" || v.Agent == "RUNNING" && (v.AgentLocation == "UNKNOWN" || v.AgentLocation == "AMBIGUOUS")) {
		v.problem("checks_incomplete", "inspect_unavailable_checks", "일부 실행 경로나 서비스 상태를 확인하지 못했습니다. 확인 불가를 정상으로 해석하지 마세요.", &exit)
	}
	if v.Code == "agent_unavailable" && v.LaunchRecord.State == "VERIFIED" {
		v.action("relaunch", "에이전트 응답이 없습니다. krasis-cli relaunch로 최근 앱을 열고 다시 확인하세요.")
	}
	if v.Connection != "CONNECTED" && v.Agent == "RUNNING" {
		v.action("check_connection", "krasis-cli status와 connection show <상대 별칭>으로 연결 설정을 확인하세요.")
	}
	if v.Protocol == "INCOMPATIBLE" {
		v.action("align_protocol", "CLI와 에이전트를 같은 배포 버전으로 맞추세요.")
	}
	return v, exit
}

func compatibleBuild(a, b string) bool { return a != "" && b != "" && a == b }
func (v *doctorResult) addCheck(name, state string) {
	v.Checks = append(v.Checks, doctorCheck{name, state})
}
func (v *doctorResult) action(code, message string) {
	for _, action := range v.NextActions {
		if action.Code == code {
			return
		}
	}
	v.NextActions = append(v.NextActions, doctorAction{code, message})
}
func (v *doctorResult) problem(code, action, message string, exit *int) {
	if *exit == ExitOK {
		*exit, v.Code = ExitUnsafeState, code
	}
	v.action(action, message)
}
func (v *doctorResult) checkMatch(name string, match bool, action, message string, exit *int) {
	state := "MATCHED"
	if !match {
		state = "MISMATCH"
		v.problem(name+"_mismatch", action, message, exit)
	}
	v.addCheck(name, state)
}
