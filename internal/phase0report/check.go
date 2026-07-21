package phase0report

import (
	"github.com/0disoft/krasis/internal/agentreport"
	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/guardreport"
)

const SchemaVersion = "krasis.lab.phase0-check/v17"

type Report struct {
	SchemaVersion string                    `json:"schemaVersion"`
	Passed        bool                      `json:"passed"`
	Issues        []string                  `json:"issues"`
	Home          agentreport.Report        `json:"home"`
	Study         agentreport.Report        `json:"study"`
	Recoveries    []guardreport.CheckResult `json:"recoveries"`
}

func Check(homePaths, studyPaths, recoveryPaths []string) Report {
	result := Report{SchemaVersion: SchemaVersion, Issues: []string{}, Recoveries: []guardreport.CheckResult{}}
	result.Home = agentreport.Check(homePaths)
	result.Study = agentreport.Check(studyPaths)
	if len(homePaths) != 2 {
		result.Issues = append(result.Issues, "home_pair_requires_two_reports")
	}
	if len(studyPaths) != 2 {
		result.Issues = append(result.Issues, "study_pair_requires_two_reports")
	}
	if !result.Home.PairPassed {
		result.Issues = append(result.Issues, "home_pair_not_passed")
	}
	if !result.Study.PairPassed {
		result.Issues = append(result.Issues, "study_pair_not_passed")
	}
	if result.Home.PairPassed && !containsAliases(result.Home.Files, "H-A", "H-B") {
		result.Issues = append(result.Issues, "home_pair_alias_mismatch")
	}
	if result.Study.PairPassed && !containsAliases(result.Study.Files, "S-A", "S-B") {
		result.Issues = append(result.Issues, "study_pair_alias_mismatch")
	}

	agentBuilds := make(map[string]string, 4)
	var commonBuild string
	var commonDuration int64
	var commonHeartbeat int64
	var commonPeerTimeout int64
	for _, file := range append(append([]agentreport.FileResult{}, result.Home.Files...), result.Study.Files...) {
		if !file.Valid {
			continue
		}
		agentBuilds[file.DeviceAlias] = file.BuildRevision
		if commonBuild == "" {
			commonBuild = file.BuildRevision
		} else if commonBuild != file.BuildRevision {
			result.Issues = appendOnce(result.Issues, "mixed_agent_build_revision")
		}
		if commonDuration == 0 {
			commonDuration = file.RequestedMillis
		} else if commonDuration != file.RequestedMillis {
			result.Issues = appendOnce(result.Issues, "mixed_agent_duration")
		}
		if commonHeartbeat == 0 {
			commonHeartbeat = file.HeartbeatMillis
		} else if commonHeartbeat != file.HeartbeatMillis {
			result.Issues = appendOnce(result.Issues, "mixed_agent_heartbeat_interval")
		}
		if commonPeerTimeout == 0 {
			commonPeerTimeout = file.PeerTimeoutMillis
		} else if commonPeerTimeout != file.PeerTimeoutMillis {
			result.Issues = appendOnce(result.Issues, "mixed_agent_peer_timeout")
		}
	}

	if len(recoveryPaths) != 4 {
		result.Issues = append(result.Issues, "recovery_requires_four_reports")
	}
	seen := make(map[string]struct{}, 4)
	for _, path := range recoveryPaths {
		checked := guardreport.Check(path, guardreport.Expectation{
			Reason: crashguard.ExitParentEOF, RequireCleanBuild: true, RequireRecovery: true,
		})
		if checked.Valid {
			if _, duplicate := seen[checked.DeviceAlias]; duplicate {
				result.Issues = appendOnce(result.Issues, "duplicate_recovery_device")
			}
			seen[checked.DeviceAlias] = struct{}{}
			if build, exists := agentBuilds[checked.DeviceAlias]; !exists {
				result.Issues = appendOnce(result.Issues, "unexpected_recovery_device")
			} else if build != checked.BuildRevision {
				result.Issues = appendOnce(result.Issues, "recovery_build_mismatch")
			}
		} else {
			result.Issues = appendOnce(result.Issues, "invalid_recovery_report")
		}
		result.Recoveries = append(result.Recoveries, checked)
	}
	if len(seen) != 4 {
		result.Issues = appendOnce(result.Issues, "missing_recovery_device")
	}
	result.Passed = len(result.Issues) == 0
	return result
}

func containsAliases(files []agentreport.FileResult, first, second string) bool {
	if len(files) != 2 {
		return false
	}
	return (files[0].DeviceAlias == first && files[1].DeviceAlias == second) ||
		(files[0].DeviceAlias == second && files[1].DeviceAlias == first)
}

func appendOnce(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
