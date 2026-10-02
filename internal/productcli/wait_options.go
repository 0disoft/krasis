package productcli

import (
	"fmt"
	"time"
)

func parseWaitOptions(args []string, command string) (time.Duration, error) {
	if (len(args) != 3 && len(args) != 5) || args[0] != "wait" || !validAlias(args[1]) || !validHex(args[2], 32) {
		return 0, usageError(fmt.Sprintf("%s wait requires ALIAS ID [--timeout DURATION]", command))
	}
	timeout := 2 * time.Minute
	if len(args) == 5 {
		var err error
		if args[3] != "--timeout" {
			return 0, usageError(fmt.Sprintf("%s wait requires --timeout DURATION", command))
		}
		timeout, err = time.ParseDuration(args[4])
		if err != nil || timeout < time.Second || timeout > 10*time.Minute {
			return 0, usageError(fmt.Sprintf("%s wait timeout must be between 1s and 10m", command))
		}
	}
	return timeout, nil
}
