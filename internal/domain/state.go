package domain

import "time"

type GameInfo struct {
	Scene string `json:"scene"`
	State string `json:"state"`
}

type Highlight struct {
	Round     uint8    `json:"round"`
	Index     uint8    `json:"index"`
	Path      string   `json:"path"`
	LengthMs  uint32   `json:"length_ms"`
	MomentsMs []uint32 `json:"moments_ms"`
}

type RosterPlayer struct {
	Name      string `json:"name"`
	PlayerID  string `json:"player_id"`
	Character string `json:"character"`
	Rank      int    `json:"rank"`
	Locked    bool   `json:"locked"`
	Local     bool   `json:"local"`
	Teammate  bool   `json:"teammate"`
}

type MatchInfo struct {
	PseudoMatchID string `json:"pseudoMatchId"`
	MatchID       string `json:"matchId"`
	Map           string `json:"map"`

	RoundNumber int    `json:"roundNumber"`
	RoundPhase  string `json:"roundPhase"`

	Roster   map[string]RosterPlayer `json:"roster"`
	KillFeed []KillFeedEntry         `json:"killFeed"`
}

type KillFeedEntry struct {
	Attacker           string `json:"attacker"`
	Victim             string `json:"victim"`
	Assist1            string `json:"assist1"`
	Assist2            string `json:"assist2"`
	Assist3            string `json:"assist3"`
	Assist4            string `json:"assist4"`
	Ult                string `json:"ult"`
	Headshot           bool   `json:"headshot"`
	Weapon             string `json:"weapon"`
	IsAttackerTeammate bool   `json:"is_attacker_teammate"`
	IsVictimTeammate   bool   `json:"is_victim_teammate"`
}

type State struct {
	UpdatedAt  time.Time           `json:"updated_at"`
	GameInfo   GameInfo            `json:"game_info"`
	MatchInfo  MatchInfo           `json:"match_info"`
	Highlights map[uint8]Highlight `json:"highlights"`
}
