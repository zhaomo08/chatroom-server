package ws

import (
	"encoding/json"
	"testing"
)

func TestMarshalWrapsKindAndPayload(t *testing.T) {
	data := Marshal("chat_message", map[string]int{"id": 42})

	var decoded struct {
		Kind    string         `json:"kind"`
		Payload map[string]int `json:"payload"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Kind != "chat_message" {
		t.Errorf("Kind = %q, want chat_message", decoded.Kind)
	}
	if decoded.Payload["id"] != 42 {
		t.Errorf("Payload[id] = %d, want 42", decoded.Payload["id"])
	}
}
