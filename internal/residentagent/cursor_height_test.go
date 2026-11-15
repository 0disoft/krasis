package residentagent

import (
	"github.com/0disoft/krasis/internal/agent"
	"testing"
)

type heightNegotiatedConnection struct {
	agent.Connection
	enabled bool
}

func (c heightNegotiatedConnection) SupportsCursorHeight() bool { return c.enabled }

func TestProductDisconnectWrapperPreservesCursorHeight(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var connection agent.Connection = &disconnectConnection{Connection: heightNegotiatedConnection{enabled: enabled}}
		capability, ok := connection.(interface{ SupportsCursorHeight() bool })
		if got := ok && capability.SupportsCursorHeight(); got != enabled {
			t.Fatalf("product connection lost height negotiation: got=%v want=%v", got, enabled)
		}
	}
	var legacy agent.Connection = &disconnectConnection{Connection: &disconnectTestConnection{}}
	if capability, ok := legacy.(interface{ SupportsCursorHeight() bool }); ok && capability.SupportsCursorHeight() {
		t.Fatal("legacy connection gained a capability it never negotiated")
	}
}
