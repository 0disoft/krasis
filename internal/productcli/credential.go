package productcli

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/transport"
)

type credentialClient interface {
	Credential(context.Context, localapi.CredentialRequest) (localapi.CredentialResult, error)
}

func parseCredential(args []string) (localapi.CredentialRequest, error) {
	request := localapi.CredentialRequest{SchemaVersion: localapi.CredentialSchemaVersion}
	switch {
	case len(args) == 2 && args[0] == "show":
		request.Action, request.Role = "SHOW", transport.AuthenticatedPeerRole(args[1])
	case len(args) == 3 && args[0] == "renew":
		request.Action, request.Role, request.ExpectedFingerprint = "RENEW", transport.AuthenticatedPeerRole(args[1]), args[2]
	}
	if !request.Valid() {
		return request, usageError("credential requires show LISTENER|CONNECTOR or renew LISTENER|CONNECTOR EXPECTED_FINGERPRINT")
	}
	return request, nil
}

func dispatchCredential(ctx context.Context, client apiClient, args []string) (any, int, error) {
	request, err := parseCredential(args)
	if err != nil {
		return nil, ExitUsage, err
	}
	configured, ok := client.(credentialClient)
	if !ok {
		return nil, ExitTransport, localapi.ErrProtocol
	}
	result, err := configured.Credential(ctx, request)
	_, code, failure := operation(localapi.OperationResult{SchemaVersion: localapi.ResultSchemaVersion, Status: result.Status, Code: result.Code}, err)
	return result, code, failure
}
