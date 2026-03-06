package handler

import (
	"encoding/json"
	"testing"
)

func TestMarshalDefaultOneBot11ConfigEscapesToken(t *testing.T) {
	t.Parallel()

	token := "qq-token-\"with\"-slashes\\and\nnewline"
	raw, err := marshalDefaultOneBot11Config(token)
	if err != nil {
		t.Fatalf("marshalDefaultOneBot11Config failed: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("generated config should be valid JSON: %v", err)
	}

	network, _ := payload["network"].(map[string]interface{})
	servers, _ := network["websocketServers"].([]interface{})
	if len(servers) != 1 {
		t.Fatalf("expected one websocket server, got %d", len(servers))
	}
	server, _ := servers[0].(map[string]interface{})
	if got, _ := server["token"].(string); got != token {
		t.Fatalf("expected token to round-trip, got %q", got)
	}
}
