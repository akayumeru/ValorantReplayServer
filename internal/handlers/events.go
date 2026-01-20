package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/persist"
	"github.com/akayumeru/valreplayserver/internal/render"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/stream"
	"github.com/akayumeru/valreplayserver/internal/valorant"
)

type EventsHandler struct {
	Store       *store.StateStore
	Hub         *stream.Hub
	Renderer    *render.Renderer
	Snapshotter *persist.Snapshotter
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

		updated, touched, applyErr := valorant.ApplyPayload(cur, body)
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
		}
	}

	h.Snapshotter.RequestSave()
	w.WriteHeader(http.StatusNoContent)
}

type HighlightRecordRequest struct {
	MatchId              string `json:"match_id"`
	MatchInternalId      string `json:"match_internal_id"`
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

	h.Store.Update(func(cur domain.State) domain.State {
		timestamps := make([]uint64, len(hlrr.RawEvents))

		for _, ev := range hlrr.RawEvents {
			timestamps = append(timestamps, ev.Time)
		}

		hl := domain.Highlight{
			MatchId:          hlrr.MatchId,
			StartTime:        hlrr.ReplayVideoStartTime,
			MediaPath:        hlrr.MediaPath,
			Duration:         hlrr.Duration,
			EventsTimestamps: timestamps,
		}

		log.Printf("Got highlight record: %#v\n", hl)

		cur.PendingHighlights = append(cur.PendingHighlights, hl)
		cur.AwaitingHighlightsCount -= uint32(len(timestamps))

		if cur.AwaitingHighlightsCount == 0 && cur.MatchInfo.RoundPhase != "combat" {
			// TODO: start replay
		}

		return cur
	})

	h.Snapshotter.RequestSave()
	w.WriteHeader(http.StatusNoContent)
}
