package productlog

func init() {
	allowed["launch_record_ready"] = true
	for _, stage := range []string{"image", "metadata", "directory", "cli", "script", "record", "unknown"} {
		allowed["launch_record_"+stage+"_failed"] = true
	}
}
