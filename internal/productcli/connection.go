package productcli

import (
	"context"

	"github.com/0disoft/krasis/internal/localapi"
)

type connectionClient interface {
	Connection(context.Context, localapi.ConnectionRequest) (localapi.ConnectionResult, error)
}

func parseConnection(args []string) (localapi.ConnectionRequest, error) {
	request := localapi.ConnectionRequest{SchemaVersion: localapi.ConnectionSchemaVersion}
	switch {
	case len(args) == 2 && args[0] == "show":
		request.Action, request.PeerAlias = "SHOW", args[1]
	case len(args) == 4 && args[0] == "set":
		request.Action, request.PeerAlias, request.Address, request.Edge = "SET", args[1], args[2], args[3]
	}
	if !request.Valid() {
		return request, usageError("connection requires show ALIAS or set ALIAS IP:PORT LEFT|RIGHT")
	}
	return request, nil
}

func dispatchConnection(ctx context.Context, client apiClient, args []string) (any, int, error) {
	request, err := parseConnection(args)
	if err != nil {
		return nil, ExitUsage, err
	}
	configured, ok := client.(connectionClient)
	if !ok {
		return nil, ExitTransport, localapi.ErrProtocol
	}
	result, err := configured.Connection(ctx, request)
	_, code, operationErr := operation(localapi.OperationResult{SchemaVersion: localapi.ResultSchemaVersion, Status: result.Status, Code: result.Code}, err)
	return result, code, operationErr
}
