package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/handlers"
	"github.com/akayumeru/valreplayserver/internal/persist"
	"github.com/akayumeru/valreplayserver/internal/render"
	"github.com/akayumeru/valreplayserver/internal/replays"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/akayumeru/valreplayserver/internal/stream"

	"github.com/rs/cors"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	initial := domain.State{
		AwaitingLastHighlight: false,
		CurrentReplayId:       0,
	}
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

	addr := "127.0.0.1:8080"
	baseUrl := &url.URL{Scheme: "http", Host: addr}

	replayBuilder := &replays.Builder{
		Store:   st,
		BaseURL: baseUrl,
	}

	events := &handlers.EventsHandler{
		Store:         st,
		Hub:           hub,
		Renderer:      renderer,
		Snapshotter:   snapshotter,
		ReplayBuilder: replayBuilder,
	}

	screens := &handlers.ScreensHandler{
		Store:    st,
		Hub:      hub,
		Renderer: renderer,
	}

	replayStreamer := &replays.Streamer{
		Store:     st,
		FFmpegBin: "ffmpeg.exe",
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

	// replays
	mux.HandleFunc("GET /replay.ts", replayStreamer.HandleStream)

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
