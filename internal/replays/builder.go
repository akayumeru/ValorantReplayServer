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

	b.Store.Update(func(cur domain.State) domain.State {
		if _, err := ReplayWindow(cur.MatchInfo); err != nil {
			return cur
		}

		if len(cur.PendingHighlights) == 0 || cur.CurrentReplayId == math.MaxUint32 {
			return cur
		}

		createdID = cur.CurrentReplayId

		newReplays := make(map[uint32][]domain.Highlight, len(cur.Replays)+1)
		for k, v := range cur.Replays {
			cp := make([]domain.Highlight, len(v))
			copy(cp, v)
			newReplays[k] = cp
		}

		// for uniqueness
		hlsPending := make(map[uint64]domain.Highlight)
		for _, hl := range cur.PendingHighlights {
			hlsPending[hl.StartTime] = hl
		}

		hlsForReplay := make([]domain.Highlight, len(hlsPending))

		for _, hl := range hlsPending {
			hlsForReplay = append(hlsForReplay, hl)
		}

		newReplays[createdID] = hlsForReplay

		cur.Replays = newReplays
		cur.PendingHighlights = nil
		cur.CurrentReplayId++

		return cur
	})

	u := *b.BaseURL
	u.Path = "/replay.ts"
	q := u.Query()
	q.Set("replay_id", fmt.Sprintf("%d", createdID))
	u.RawQuery = q.Encode()

	return createdID, u.String()
}
