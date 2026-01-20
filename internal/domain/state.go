package domain

import "time"

type PlayerPick struct {
	PlayerName string `json:"player_name"`
	Agent      string `json:"agent"`
	Locked     bool   `json:"locked"`
}

type Highlight struct {
	Round     uint8    `json:"round"`
	Index     uint8    `json:"index"`
	Path      string   `json:"path"`
	LengthMs  uint32   `json:"length_ms"`
	MomentsMs []uint32 `json:"moments_ms"`
}

type MatchInfo struct {
	ID       string    `json:"id"`
	Map      string    `json:"map"`
	State    string    `json:"state"`
	StartsAt time.Time `json:"starts_at"`
}

type State struct {
	UpdatedAt  time.Time           `json:"updated_at"`
	Picks      []PlayerPick        `json:"picks"`
	Match      MatchInfo           `json:"match"`
	Highlights map[uint8]Highlight `json:"highlights"`
}
