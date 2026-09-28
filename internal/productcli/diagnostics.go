package productcli

import "github.com/0disoft/krasis/internal/productlog"

const diagnosticsExportSchema = "krasis.cli.diagnostics-export/v1"

type diagnosticsExportResult struct {
	SchemaVersion string `json:"schemaVersion"`
	Code          string `json:"code"`
	Path          string `json:"path,omitempty"`
}

func exportDiagnostics(localAppData string) (diagnosticsExportResult, int) {
	result := diagnosticsExportResult{SchemaVersion: diagnosticsExportSchema, Code: "save_failed"}
	path, err := productlog.Export(localAppData)
	if err != nil {
		return result, ExitInternal
	}
	result.Code, result.Path = "saved", path
	return result, ExitOK
}
