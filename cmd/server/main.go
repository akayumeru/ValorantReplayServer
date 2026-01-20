package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/handlers"
	"github.com/akayumeru/valreplayserver/internal/persist"
	"github.com/akayumeru/valreplayserver/internal/render"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/stream"

	"github.com/rs/cors"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	initial := domain.State{}
	st := store.NewStateStore(initial)

	snapshotter := persist.NewSnapshotter("./state.json", st, 3*time.Second)
	if loaded, ok, err := snapshotter.LoadOnStartup(); err != nil {
		log.Fatalf("snapshot load failed: %v", err)
	} else if ok {
		st.Replace(loaded)
	}

	hub := stream.NewHub()

	renderer, err := render.NewRenderer()
	if err != nil {
		log.Fatalf("renderer init failed: %v", err)
	}

	events := &handlers.EventsHandler{
		Store:       st,
		Hub:         hub,
		Renderer:    renderer,
		Snapshotter: snapshotter,
	}
	screens := &handlers.ScreensHandler{
		Store:    st,
		Hub:      hub,
		Renderer: renderer,
	}

	go func() {
		_ = snapshotter.Run(ctx)
	}()

	mux := http.NewServeMux()

	// events
	mux.HandleFunc("POST /events/game_event", events.HandleGameEvent)
	mux.HandleFunc("POST /events/highlight_record", events.HandleHighlightRecord)

	// screens
	mux.HandleFunc("GET /screens/player_picks", screens.PlayerPicksPage)
	mux.HandleFunc("GET /screens/match_info", screens.MatchInfoPage)

	// streams
	mux.HandleFunc("GET /screens/player_picks/stream", screens.PlayerPicksStream)
	mux.HandleFunc("GET /screens/match_info/stream", screens.MatchInfoStream)

	handler := cors.AllowAll().Handler(mux)

	srv := &http.Server{
		Addr:              "127.0.0.1:8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = srv.Shutdown(shutdownCtx)
}
