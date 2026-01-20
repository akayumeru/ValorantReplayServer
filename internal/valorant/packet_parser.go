package valorant

import (
	"encoding/json"
)

type RawEvent struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type Envelope struct {
	Events    []RawEvent                 `json:"events"`
	MatchInfo map[string]json.RawMessage `json:"match_info"`
	GameInfo  map[string]json.RawMessage `json:"game_info"`
}

func ParseEnvelope(b []byte) (Envelope, map[string]json.RawMessage, error) {
	var env Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return Envelope{}, nil, err
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal(b, &root)
	return env, root, nil
}
