package ws

import "encoding/json"

// Envelope wraps every payload pushed over the WebSocket so the client can
// tell push types apart (chat message vs. call invite, etc.) without
// guessing from the JSON shape.
type Envelope struct {
	Kind    string `json:"kind"`
	Payload any    `json:"payload"`
}

// Marshal builds an Envelope around payload. Marshaling failures are dropped
// rather than propagated: every current payload type (Message, call
// notifications) is a plain struct that cannot fail to marshal, and callers
// already treat a send as best-effort (the receiving client may be offline).
func Marshal(kind string, payload any) []byte {
	data, err := json.Marshal(Envelope{Kind: kind, Payload: payload})
	if err != nil {
		return nil
	}
	return data
}
