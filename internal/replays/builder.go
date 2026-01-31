package replays

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/store"
)

type Builder struct {
	Store   *store.StateStore
	BaseURL *url.URL
}

func (b *Builder) CreateReplay() (uint32, string, error) {
	var createdID uint32
	var notCreated bool

	b.Store.Update(func(cur domain.State) domain.State {
		notCreated = true

		if len(cur.ReplayState.PendingHighlights) == 0 || cur.ReplayState.CurrentReplayId == math.MaxUint32 {
			return cur
		}

		createdID = cur.ReplayState.CurrentReplayId

		var hlRounds []uint64
		for _, hl := range cur.ReplayState.PendingHighlights {
			if hl.Round != 0 {
				hlRounds = append(hlRounds, hl.Round)
			}
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

		var roundNumber = 0
		if cur.MatchInfo.CurrentRound != nil {
			roundNumber = cur.MatchInfo.CurrentRound.Number
		} else {
			for _, round := range cur.MatchInfo.Rounds {
				if roundNumber <= round.Number {
					roundNumber = round.Number
				}
			}
		}

		replay := domain.Replay{
			RoundNumber: roundNumber,
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
		return 0, "", errors.New("replay was not created")
	}

	u := *b.BaseURL
	u.Path = "/replay.ts"
	q := u.Query()
	q.Set("replay_id", fmt.Sprintf("%d", createdID))
	u.RawQuery = q.Encode()

	log.Printf("Created replay with id %d (%s)\n", createdID, u.String())

	return createdID, u.String(), nil
}
