package guardchild

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/guardreport"
)

func TestRunWritesDisarmEvidenceBeforeExit(t *testing.T) {
	output := filepath.Join(t.TempDir(), "guard.json")
	serverInput, clientOutput := io.Pipe()
	clientInput, serverOutput := io.Pipe()
	done := make(chan int, 1)
	go func() {
		done <- Run([]string{"--device", "H-A", "--output", output}, serverInput, serverOutput, io.Discard)
	}()
	client, err := crashguard.NewClient(clientOutput, clientInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Disarm(); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != 0 {
		t.Fatalf("Run() exit = %d", code)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var report guardreport.Report
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != guardreport.SchemaVersion || report.Reason != string(crashguard.ExitDisarm) || !report.Passed {
		t.Fatalf("guard report = %+v", report)
	}
}

func TestRunRejectsIncompleteInternalArguments(t *testing.T) {
	if code := Run([]string{"--device", "H-A"}, io.NopCloser(nilReader{}), io.Discard, io.Discard); code != 2 {
		t.Fatalf("Run() exit = %d", code)
	}
}

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }
