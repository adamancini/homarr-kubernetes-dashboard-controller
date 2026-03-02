package controller

import "encoding/json"

func marshalJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func unmarshalJSON(data json.RawMessage, v any) error {
	return json.Unmarshal(data, v)
}
