//go:build windows

package productcli

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unsafe"

	"github.com/0disoft/krasis/internal/launchrecord"
)

func TestDoctorKeepsUIProcessPresenceWithoutVerifiedRecord(t *testing.T) {
	build := strings.Repeat("a", 40)
	record := launchrecord.Record{Executable: `C:\product\krasis.exe`, Generation: 118, Revision: build}
	for _, test := range []struct {
		name             string
		paths            []string
		unknown          bool
		recordErr        error
		imageBuild       string
		state, pathState string
		generation       int
	}{
		{"verified", []string{record.Executable}, false, nil, build, "RUNNING", "MATCHED", 118},
		{"missing_record", []string{record.Executable}, false, launchrecord.ErrMissing, build, "RUNNING_UNVERIFIED", "UNKNOWN", 0},
		{"invalid_record", []string{record.Executable}, false, launchrecord.ErrInvalid, build, "RUNNING_UNVERIFIED", "MISMATCH", 0},
		{"unreadable_record", []string{record.Executable}, false, errors.New("private path"), build, "RUNNING_UNVERIFIED", "UNKNOWN", 0},
		{"different_image", []string{`C:\other\krasis.exe`}, false, nil, build, "RUNNING_UNVERIFIED", "MISMATCH", 0},
		{"different_build", []string{record.Executable}, false, nil, strings.Repeat("b", 40), "RUNNING_UNVERIFIED", "MISMATCH", 0},
		{"not_running", nil, false, launchrecord.ErrMissing, build, "NOT_RUNNING", "", 0},
		{"query_unknown", []string{record.Executable}, true, nil, build, "UNKNOWN", "", 0},
		{"multiple_images", []string{record.Executable, `C:\other\krasis.exe`}, false, nil, build, "UNKNOWN", "", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ui := doctorUIComponent(test.paths, test.unknown, record, test.recordErr, func(string) string { return test.imageBuild })
			if ui.State != test.state || ui.PathStatus != test.pathState || ui.Generation != test.generation {
				t.Fatalf("ui=%+v", ui)
			}
		})
	}
}

func TestDoctorServiceStringsStayInsideAlignedBuffer(t *testing.T) {
	words := make([]uint64, 4)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*8)
	binary.LittleEndian.PutUint16(buffer[8:10], 'A')
	ptr := (*uint16)(unsafe.Pointer(&buffer[8]))
	if value, ok := doctorServiceString(buffer, ptr); !ok || value != "A" {
		t.Fatal("native service field rejected")
	}
	var other uint16
	if _, ok := doctorServiceString(buffer, &other); ok {
		t.Fatal("foreign native pointer accepted")
	}
	if _, ok := doctorServiceString(buffer, nil); ok {
		t.Fatal("missing native field accepted")
	}
	for i := 8; i < len(buffer); i += 2 {
		binary.LittleEndian.PutUint16(buffer[i:i+2], 'B')
	}
	if _, ok := doctorServiceString(buffer, ptr); ok {
		t.Fatal("unterminated native field read past buffer")
	}
}

func TestDoctorCancelledHostInspectionRemainsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	host := inspectDoctorHost(ctx, t.TempDir(), "development")
	if host.Service.State != "UNKNOWN" || host.AgentLocation != "UNKNOWN" {
		t.Fatal("cancelled native checks claimed healthy")
	}
}

func TestDoctorProcessLocationNeverReopensInstalledImage(t *testing.T) {
	installed := `C:\Program Files\Krasis\Input\krasis-agent.exe`
	for _, state := range []string{"VERIFIED", "UNKNOWN", "INVALID"} {
		observed := doctorComponent{State: state, Build: strings.Repeat("a", 40)}
		build := doctorAgentImageBuild(strings.ToUpper(installed), installed, observed, func(string) string {
			t.Fatal("installed image reopened outside observation guard")
			return ""
		})
		if (state == "VERIFIED" && build != observed.Build) || (state != "VERIFIED" && build != "") {
			t.Fatal("unverified observation became an installed build", state, build)
		}
	}
	read := 0
	if build := doctorAgentImageBuild(`C:\bundle\krasis-agent.exe`, installed, doctorComponent{}, func(string) string { read++; return "portable-build" }); build != "portable-build" || read != 1 {
		t.Fatal("portable image inspection was lost", build, read)
	}
}
