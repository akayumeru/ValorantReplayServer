package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/persist"
	"github.com/akayumeru/valreplayserver/internal/render"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/stream"
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
	var ev GameEvent
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	next := h.Store.Update(func(cur domain.State) domain.State {
		switch ev.Type {
		case "player_pick":
			picks := append([]domain.PlayerPick(nil), cur.Picks...)
			picks = append(picks, domain.PlayerPick{
				PlayerName: ev.PlayerName,
				Agent:      ev.Agent,
				Locked:     ev.Locked,
			})
			cur.Picks = picks
		}
		return cur
	})

	h.Hub.Publish("player_picks", h.Renderer.RenderPlayerPicksFragment(next))

	h.Snapshotter.RequestSave()

	w.WriteHeader(http.StatusNoContent)
}
