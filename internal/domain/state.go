package domain

import "time"

type GameInfo struct {
	Scene string `json:"scene"`
	State string `json:"state"`
}

type Highlight struct {
	MatchId          string   `json:"matchId"`
	StartTime        uint64   `json:"startTime"`
	Round            uint64   `json:"round"`
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

	CurrentRound *Round
	Rounds       map[int]*Round `json:"rounds"`

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

type ReplayState struct {
	CurrentReplayId   uint32            `json:"currentReplayId"`
	PendingHighlights []*Highlight      `json:"pendingHighlights"`
	Replays           map[uint32]Replay `json:"replays"`
}

type Round struct {
	Number    int       `json:"number"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`

	HighlightsCount uint32 `json:"highlightsCount"`

	LastPhase      string    `json:"lastPhase"`
	PhaseStartedAt time.Time `json:"phaseStartedAt"`
}

type PlayerInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type Replay struct {
	RoundNumber int          `json:"roundNumber"`
	Highlights  []*Highlight `json:"highlights"`
}

type State struct {
	UpdatedAt   time.Time   `json:"updatedAt"`
	PlayerInfo  PlayerInfo  `json:"playerInfo"`
	GameInfo    GameInfo    `json:"gameInfo"`
	MatchInfo   MatchInfo   `json:"matchInfo"`
	ReplayState ReplayState `json:"replayState"`
}
