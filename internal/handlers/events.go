package handlers

import (
	"io"
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

type GameEvent struct {
	Type       string `json:"type"`
	PlayerName string `json:"playerName,omitempty"`
	Agent      string `json:"agent,omitempty"`
	Locked     bool   `json:"locked,omitempty"`
}

type HighlightRecordedEvent struct {
	ID       string `json:"id"`
	ClipPath string `json:"clipPath"`
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
