package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/highlighter"
	"github.com/akayumeru/valreplayserver/internal/persist"
	"github.com/akayumeru/valreplayserver/internal/render"
	"github.com/akayumeru/valreplayserver/internal/replays"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/stream"
	"github.com/akayumeru/valreplayserver/internal/utils"
	"github.com/akayumeru/valreplayserver/internal/valorant"
)

type EventsHandler struct {
	Store         *store.StateStore
	Hub           *stream.Hub
	Renderer      *render.Renderer
	Snapshotter   *persist.Snapshotter
	ReplayBuilder *replays.Builder
	Highligher    *highlighter.Highlighter
}

func (h *EventsHandler) HandleGameEvent(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}

	var topics []string
	next := h.Store.Update(func(curState domain.State) domain.State {
		cur := curState

		updated, touched, applyErr := valorant.ApplyPayload(cur, body, h.Highligher.RecordHighlight)
		if applyErr != nil {
			return cur
		}

		topics = touched.List()
		return updated
	})

	for _, t := range topics {
		switch t {
		case "player_picks":
			h.Hub.Publish(t, h.Renderer.RenderPlayerPicksFragment(next))
		case "match_info":
			h.Hub.Publish(t, h.Renderer.RenderMatchInfoFragment(next))
		case "replays":
			h.tryToCreateReplay(next)
		}
	}

	h.Snapshotter.RequestSave()
	w.WriteHeader(http.StatusNoContent)
}

func (h *EventsHandler) tryToCreateReplay(cur domain.State) {
	_, replayUrl := h.ReplayBuilder.CreateReplay()
	if replayUrl != "" {
		log.Printf("New replay created, replay url: %s\n", replayUrl)
	}
	// TODO: start replay
}

type HighlightRecordRequest struct {
	MatchId              string `json:"match_id"`
	MatchInternalId      string `json:"match_internal_id"`
	StartTime            uint64 `json:"start_time"`
	ReplayVideoStartTime uint64 `json:"replay_video_start_time"`
	Duration             uint64 `json:"duration"`
	RawEvents            []struct {
		Time uint64 `json:"time"`
	} `json:"raw_events"`
	MediaPath     string `json:"media_path"`
	ThumbnailPath string `json:"thumbnail_path"`
}

func (h *EventsHandler) HandleHighlightRecord(w http.ResponseWriter, r *http.Request) {
	var hlrr HighlightRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&hlrr); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	var canCreateReplay = false

	next := h.Store.Update(func(cur domain.State) domain.State {
		timestamps := make([]uint64, len(hlrr.RawEvents))

		for _, ev := range hlrr.RawEvents {
			if ev.Time != 0 {
				timestamps = append(timestamps, ev.Time)
			}
		}

		hl := &domain.Highlight{
			MatchId:          hlrr.MatchId,
			StartTime:        hlrr.StartTime,
			MediaPath:        hlrr.MediaPath,
			Duration:         hlrr.Duration,
			EventsTimestamps: timestamps,
		}

		var roundNumber uint64 = 0

		for _, round := range cur.MatchInfo.Rounds {
			if round.StartedAt.UnixMilli() >= int64(hl.StartTime) && (round.EndedAt.UnixMilli() > int64(hl.StartTime)) {
				roundNumber = uint64(round.Number)
				break
			}
		}

		hl.Round = roundNumber

		utils.DebugLog("Got highlight record", hl)
		cur.ReplayState.PendingHighlights = append(cur.ReplayState.PendingHighlights, hl)

		if uint64(cur.MatchInfo.CurrentRound.Number) == roundNumber+1 && len(cur.ReplayState.PendingHighlights) > 0 {
			canCreateReplay = true
		}

		return cur
	})

	if canCreateReplay {
		h.tryToCreateReplay(next)
	}

	h.Snapshotter.RequestSave()
	w.WriteHeader(http.StatusNoContent)
}
