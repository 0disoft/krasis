package logindiag

import (
	"encoding/json"
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestApprovalFailurePreservesStageAndNativeWithoutSecrets(t *testing.T) {
	err := At("prepare", At("local_credential", errors.Join(errors.New("SECRET/SID/path"), syscall.Errno(5))))
	s := New("prepare", 3, err)
	data, _ := json.Marshal(s)
	if !s.Valid() || s.Stage != "local_credential" || !s.Win32Known || s.Win32Error != 5 || strings.Contains(string(data), "SECRET") {
		t.Fatal("invalid failure projection", string(data))
	}
	if !errors.Is(err, syscall.Errno(5)) {
		t.Fatal("native cause lost")
	}
	s.Stage = "SECRET/path"
	if s.Valid() {
		t.Fatal("private stage accepted")
	}
	if s = New("prepare", 3, errors.New("unknown")); !s.Valid() || s.Win32Known {
		t.Fatal("unknown error invented native result")
	}
}
