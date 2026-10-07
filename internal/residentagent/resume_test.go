package residentagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestManualStopSurvivesAgentReplacementAndExplicitConnectClearsIt(t *testing.T) {
	base := t.TempDir()
	newAgent := func() *Operations {
		o := testOperations(t, func(ctx context.Context, _ string, _ agent.SupervisorObserver) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})
		o.localAppData = base
		o.trust = &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Alias: "peer", Fingerprint: strings.Repeat("b", 64)}}}}
		return o
	}
	old := newAgent()
	old.Disconnect(context.Background(), localapi.OperationRequest{})
	if err := old.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	fresh := newAgent()
	fresh.Disconnect(context.Background(), localapi.OperationRequest{MaintenanceStop: true})
	result, err := fresh.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer", AutomaticResume: true})
	if err != nil || result.Code != "resume_cancelled" {
		t.Fatal("replacement lost manual stop", result, err)
	}
	result, err = fresh.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
	if err != nil || result.Status != "COMPLETED" {
		t.Fatal("explicit connection blocked", result, err)
	}
	saved, err := appsettings.LoadResume(base)
	if err != nil || saved.ManualStop || saved.Alias != "peer" {
		t.Fatal("explicit connection did not replace stop", saved, err)
	}
	if err := fresh.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := newAgent()
	result, err = after.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer", AutomaticResume: true})
	if err != nil || result.Status != "COMPLETED" {
		t.Fatal("explicit connection did not reenable automatic resume", result, err)
	}
	if err := after.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUnreadableResumeBlocksAutomaticConnectAndReportsStopSaveFailure(t *testing.T) {
	o := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error {
		t.Fatal("connected with unreadable stop state")
		return nil
	})
	base := t.TempDir()
	if err := appsettings.SaveResume(base, appsettings.Resume{ManualStop: true}); err != nil {
		t.Fatal(err)
	}
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Root, "launch", "resume.json"), []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	o.localAppData = base
	result, err := o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer", AutomaticResume: true})
	if err != nil || result.Code != "resume_unavailable" {
		t.Fatal(result, err)
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	o.localAppData = file
	result, err = o.Disconnect(context.Background(), localapi.OperationRequest{})
	if err != nil || result.Code != "resume_save_failed" || result.Status != "FAILED" || !o.manualStop {
		t.Fatal("failed persistence advertised success", result, err)
	}
	if err := o.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionResumeSurvivesShutdownButNotManualDisconnect(t *testing.T) {
	for _, manual := range []bool{false, true} {
		o := testOperations(t, func(ctx context.Context, _ string, _ agent.SupervisorObserver) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})
		o.localAppData = t.TempDir()
		o.trust = &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Alias: "peer", Fingerprint: strings.Repeat("b", 64)}}}}
		result, err := o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
		if err != nil || result.Status != "COMPLETED" {
			t.Fatal(result, err)
		}
		resume, err := appsettings.LoadResume(o.localAppData)
		if err != nil || resume.Alias != "peer" {
			t.Fatal(resume, err)
		}
		if manual {
			_, err = o.Disconnect(context.Background(), localapi.OperationRequest{})
		} else {
			err = o.Close(context.Background())
		}
		if err != nil {
			t.Fatal(err)
		}
		resume, err = appsettings.LoadResume(o.localAppData)
		if err != nil || (resume.Alias == "") != manual {
			t.Fatal(resume, err)
		}
	}
}

func TestConnectionSaveFailureIsReportedWithoutCancelingTheConnection(t *testing.T) {
	started := make(chan struct{}, 2)
	o := testOperations(t, func(ctx context.Context, _ string, _ agent.SupervisorObserver) error {
		started <- struct{}{}
		<-ctx.Done()
		return context.Cause(ctx)
	})
	o.trust = &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Alias: "peer", Fingerprint: strings.Repeat("b", 64)}}}}
	occupied := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(occupied, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	o.localAppData = occupied
	result, err := o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
	if err != nil || result.Status != "COMPLETED" || result.Code != "connect_started_resume_save_failed" {
		t.Fatal("connection or persistence result was lost", result, err)
	}
	<-started
	for range 2 {
		status, err := o.Status(context.Background())
		if err != nil || status.ResumeState != "FAILED" || status.State != "CONNECTING" {
			t.Fatal("status hid save failure or canceled connection", status, err)
		}
	}
	o.Disconnect(context.Background(), localapi.OperationRequest{})
	o.localAppData = t.TempDir()
	result, err = o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
	if err != nil || result.Code != "connect_started" {
		t.Fatal(result, err)
	}
	<-started
	if status, _ := o.Status(context.Background()); status.ResumeState != "SAVED" {
		t.Fatal("verified successful save did not clear warning", status)
	}
	if err := o.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
