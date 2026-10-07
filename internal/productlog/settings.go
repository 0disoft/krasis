package productlog

func init() {
	allowed["settings_restore_all_verified"] = true
	for _, stage := range []string{"startup", "recovery", "save"} {
		allowed["settings_change_"+stage+"_failed"] = true
	}
	for _, stage := range []string{"startup", "recovery"} {
		for _, result := range []string{"query_failed", "write_failed", "mismatch", "verified"} {
			allowed["settings_restore_"+stage+"_"+result] = true
		}
	}
}
