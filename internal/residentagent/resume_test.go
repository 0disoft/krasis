package residentagent

import (
	"context"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/truststore"
)

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
