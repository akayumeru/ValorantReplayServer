package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/akayumeru/valreplayserver/internal/render"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/stream"
)

type ScreensHandler struct {
	Store    *store.StateStore
	Hub      *stream.Hub
	Renderer *render.Renderer
}

func (h *ScreensHandler) PlayerPicksPage(w http.ResponseWriter, r *http.Request) {
	page, err := h.Renderer.RenderPlayerPicksPage(h.Store.Get())
	if err != nil {
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (h *ScreensHandler) MatchInfoPage(w http.ResponseWriter, r *http.Request) {
	page, err := h.Renderer.RenderMatchInfoPage(h.Store.Get())
	if err != nil {
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (h *ScreensHandler) PlayerPicksStream(w http.ResponseWriter, r *http.Request) {
	h.serveSSE(w, r, "player_picks")
}

func (h *ScreensHandler) MatchInfoStream(w http.ResponseWriter, r *http.Request) {
	h.serveSSE(w, r, "match_info")
}

func (h *ScreensHandler) serveSSE(w http.ResponseWriter, r *http.Request, topic string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := h.Hub.Subscribe(topic)
	defer cancel()

	var first []byte
	switch topic {
	case "player_picks":
		first = h.Renderer.RenderPlayerPicksFragment(h.Store.Get())
	case "match_info":
		first = h.Renderer.RenderMatchInfoFragment(h.Store.Get())
	}
	fmt.Fprintf(w, "event: update\n")
	fmt.Fprintf(w, "data: %s\n\n", first)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case payload, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: update\n")
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
