package guardchild

import (
	"fmt"
	"io"

	"github.com/0disoft/krasis/internal/buildinfo"
	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/evidence"
	"github.com/0disoft/krasis/internal/guardreport"
	"github.com/0disoft/krasis/internal/lab"
	"github.com/0disoft/krasis/internal/wininput"
)

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	device, output, err := parse(args)
	if err != nil {
		fmt.Fprintln(stderr, "invalid crash guard configuration")
		return 2
	}
	if err := evidence.ValidateJSONOutputPath(output); err != nil {
		fmt.Fprintln(stderr, "invalid crash guard output")
		return 2
	}
	result, serveErr := crashguard.ServeWithResult(stdin, stdout, wininput.NewInputAdapter())
	report := guardreport.New(device, buildinfo.Read(), result, serveErr)
	if err := evidence.WriteJSONExclusive(output, report); err != nil {
		fmt.Fprintln(stderr, "crash guard evidence write failed")
		return 1
	}
	if serveErr != nil {
		return 1
	}
	return 0
}

func parse(args []string) (string, string, error) {
	if len(args) != 4 || args[0] != "--device" || args[2] != "--output" {
		return "", "", fmt.Errorf("device and output are required")
	}
	if !lab.ValidProbeDeviceAlias(args[1]) || args[3] == "" {
		return "", "", fmt.Errorf("device or output is invalid")
	}
	return args[1], args[3], nil
}
