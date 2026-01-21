package replays

import (
	"fmt"
	"math"
	"net/url"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/store"
)

type Builder struct {
	Store   *store.StateStore
	BaseURL *url.URL
}

func (b *Builder) CreateReplay() (uint32, string) {
	var createdID uint32
	var notCreated bool

	b.Store.Update(func(cur domain.State) domain.State {
		notCreated = true
		if _, err := ReplayWindow(cur.MatchInfo); err != nil {
			return cur
		}

		if len(cur.ReplayState.PendingHighlights) == 0 || cur.ReplayState.CurrentReplayId == math.MaxUint32 {
			return cur
		}

		createdID = cur.ReplayState.CurrentReplayId

		hlMomentsTotal := 0
		var hlRounds []uint64
		for _, hl := range cur.ReplayState.PendingHighlights {
			hlMomentsTotal += len(hl.EventsTimestamps)
			if hl.Round != 0 {
				hlRounds = append(hlRounds, hl.Round)
			}
		}

		var roundMomentsTotal uint32 = 0
		for _, roundNumber := range hlRounds {
			roundMomentsTotal += cur.MatchInfo.Rounds[int(roundNumber)-1].HighlightsCount
		}

		if uint32(hlMomentsTotal) < roundMomentsTotal {
			return cur
		}

		// for uniqueness
		hlsPending := make(map[uint64]*domain.Highlight)
		for _, hl := range cur.ReplayState.PendingHighlights {
			hlsPending[hl.StartTime] = hl
		}

		hlsForReplay := make([]*domain.Highlight, len(hlsPending))

		for _, hl := range hlsPending {
			hlsForReplay = append(hlsForReplay, hl)
		}

		replay := domain.Replay{
			RoundNumber: cur.MatchInfo.CurrentRound.Number,
			Highlights:  hlsForReplay,
		}

		if cur.ReplayState.Replays == nil {
			cur.ReplayState.Replays = make(map[uint32]domain.Replay)
		}

		cur.ReplayState.Replays[createdID] = replay
		cur.ReplayState.PendingHighlights = nil
		cur.ReplayState.CurrentReplayId++
		notCreated = false

		return cur
	})

	if notCreated {
		return 0, ""
	}

	u := *b.BaseURL
	u.Path = "/replay.ts"
	q := u.Query()
	q.Set("replay_id", fmt.Sprintf("%d", createdID))
	u.RawQuery = q.Encode()

	return createdID, u.String()
}
