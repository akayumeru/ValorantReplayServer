package handlers

import (
	"context"
	"io"
	"log"
	"net/http"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/highlighter"
	"github.com/akayumeru/valreplayserver/internal/obs"
	"github.com/akayumeru/valreplayserver/internal/persist"
	"github.com/akayumeru/valreplayserver/internal/render"
	"github.com/akayumeru/valreplayserver/internal/replays"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/stream"
	"github.com/akayumeru/valreplayserver/internal/valorant"
)

type EventsHandler struct {
	Store         *store.StateStore
	Hub           *stream.Hub
	Renderer      *render.Renderer
	Snapshotter   *persist.Snapshotter
	ReplayBuilder *replays.Builder
	Highligher    *highlighter.Highlighter
	ObsController *obs.Controller
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
		case "highlight":
			h.Highligher.RecordHighlight()
		case "trigger_replay":
			h.CreateReplayAndStart()
		case "start_replay_buffer":
			h.ObsController.StopReplay()
			h.ObsController.StartReplayBuffer()
		}
	}

	h.Snapshotter.RequestSave()
	w.WriteHeader(http.StatusNoContent)
}

func (h *EventsHandler) CreateReplayAndStart() {
	h.Highligher.FlushIfHasHighlightsNow(context.Background())

	replayId, _, err := h.ReplayBuilder.CreateReplay()
	if err != nil {
		log.Println(err)
		return
	}

	h.ObsController.StartReplay(replayId)
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
