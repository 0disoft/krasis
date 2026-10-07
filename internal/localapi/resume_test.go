package localapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResumeConnectionMarksOneAuthenticatedConnectRequest(t *testing.T) {
	calls := 0
	client := &Client{credentials: ClientCredentials{Endpoint: Endpoint{Address: "127.0.0.1:1234"}, Token: strings.Repeat("a", 64)}, http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var v OperationRequest
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			t.Fatal(err)
		}
		if r.Method != "POST" || r.URL.Path != "/v1/connect" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) || v.PeerAlias != "peer" || !v.AutomaticResume || v.SchemaVersion != OperationSchemaVersion {
			t.Fatal("incorrect resume request")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"schemaVersion":"krasis.agent.operation-result/v1","status":"REJECTED","code":"resume_cancelled"}`))}, nil
	})}}
	v, err := client.ResumeConnection(context.Background(), "peer")
	if err != nil || v.Code != "resume_cancelled" || calls != 1 {
		t.Fatal(v, err, calls)
	}
}

func TestMaintenancePauseCompatibilityAndAuthentication(t *testing.T) {
	for _, generation := range []int{225, 226} {
		calls := 0
		client := &Client{credentials: ClientCredentials{Endpoint: Endpoint{Address: "127.0.0.1:1234"}, Token: strings.Repeat("a", 64)}, http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) {
				t.Fatal("unauthenticated pause")
			}
			var body []byte
			if r.URL.Path == "/v1/runtime" && r.Method == "GET" {
				body, _ = json.Marshal(RuntimeInfo{SchemaVersion: RuntimeSchemaVersion, Protocol: AgentProtocol, Generation: generation, Instance: strings.Repeat("b", 32)})
			} else {
				var v OperationRequest
				if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
					t.Fatal(err)
				}
				if calls != 2 || r.URL.Path != "/v1/disconnect" || r.Method != "POST" || v.MaintenanceStop != (generation >= 226) || v.AutomaticResume || v.SchemaVersion != OperationSchemaVersion {
					t.Fatal("incorrect maintenance request", r.URL, v)
				}
				body, _ = json.Marshal(OperationResult{SchemaVersion: ResultSchemaVersion, Status: "COMPLETED", Code: "disconnected"})
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})}}
		if result, err := client.PauseForMaintenance(context.Background()); err != nil || result.Status != "COMPLETED" || calls != 2 {
			t.Fatal(generation, result, err, calls)
		}
	}
}

func TestMaintenanceStopFlagIsRestrictedToDisconnect(t *testing.T) {
	if err := validateOperationRequest("disconnect", OperationRequest{MaintenanceStop: true}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"connect", "pair", "trust_revoke"} {
		if err := validateOperationRequest(kind, OperationRequest{PeerAlias: "peer", MaintenanceStop: true}); err == nil {
			t.Fatal("maintenance flag accepted on", kind)
		}
	}
}

func TestAutomaticResumeFlagIsRestrictedToConnect(t *testing.T) {
	if err := validateOperationRequest("connect", OperationRequest{PeerAlias: "peer", AutomaticResume: true}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"disconnect", "pair", "trust_revoke"} {
		if err := validateOperationRequest(kind, OperationRequest{AutomaticResume: true}); err == nil {
			t.Fatal("resume flag accepted on", kind)
		}
	}
}
