package launcher

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	ReportBundleSchemaVersion = "krasis.launcher.report-bundle/v1"
	maxLauncherReportBytes    = 2 << 20
)

type reportBundle struct {
	SchemaVersion string          `json:"schemaVersion"`
	Agent         json.RawMessage `json:"agent"`
	Guard         json.RawMessage `json:"guard"`
}

func BundleReports(agentPath, guardPath string) ([]byte, error) {
	agent, err := readJSONObject(agentPath)
	if err != nil {
		return nil, fmt.Errorf("read agent report: %w", err)
	}
	guard, err := readJSONObject(guardPath)
	if err != nil {
		return nil, fmt.Errorf("read guard report: %w", err)
	}
	payload, err := json.MarshalIndent(reportBundle{
		SchemaVersion: ReportBundleSchemaVersion,
		Agent:         agent,
		Guard:         guard,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func readJSONObject(path string) (json.RawMessage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxLauncherReportBytes {
		return nil, errors.New("report must be a regular bounded file")
	}
	payload := make([]byte, info.Size())
	if _, err := io.ReadFull(file, payload); err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&object); err != nil || object == nil {
		if err == nil {
			err = errors.New("report root must be an object")
		}
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("report contains multiple JSON values")
		}
		return nil, err
	}
	return json.RawMessage(payload), nil
}
