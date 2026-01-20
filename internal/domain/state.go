package domain

import "time"

type GameInfo struct {
	Scene string `json:"scene"`
	State string `json:"state"`
}

type Highlight struct {
	MatchId          string   `json:"matchId"`
	StartTime        uint64   `json:"startTime"`
	MediaPath        string   `json:"mediaPath"`
	Duration         uint64   `json:"duration"`
	EventsTimestamps []uint64 `json:"eventsTimestamps"`
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

	RoundNumber         int       `json:"roundNumber"`
	RoundStartedAt      time.Time `json:"roundStartedAt"`
	RoundPhase          string    `json:"roundPhase"`
	RoundPhaseStartedAt time.Time `json:"roundPhaseStartedAt"`

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
	IsAttackerTeammate bool   `json:"isAttackerTeammate"`
	IsVictimTeammate   bool   `json:"isVictimTeammate"`
}

type State struct {
	UpdatedAt               time.Time              `json:"updatedAt"`
	GameInfo                GameInfo               `json:"gameInfo"`
	MatchInfo               MatchInfo              `json:"matchInfo"`
	AwaitingHighlightsCount uint32                 `json:"awaitingHighlightsCount"`
	PendingHighlights       []Highlight            `json:"pendingHighlights"`
	CurrentReplayId         uint32                 `json:"currentReplayId"`
	Replays                 map[uint32][]Highlight `json:"replays"`
}
