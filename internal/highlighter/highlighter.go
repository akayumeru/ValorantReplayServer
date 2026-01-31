package highlighter

import (
	"context"
	"errors"
	"log"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/persist"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/andreykaipov/goobs"
)

type Highlight = domain.Highlight

type bufferSession struct {
	id       uint64
	firstAt  time.Time
	lastAt   time.Time
	kills    []time.Time
	timer    *time.Timer
	closed   bool
	savedReq bool
}

type pendingSave struct {
	sessionID    uint64
	requestedAt  time.Time
	bufferStart  time.Time // requestedAt - bufferLen
	eventsOffset []uint64  // offsets from bufferStart in ms
}

type Config struct {
	BufferLen   time.Duration
	PreWindow   time.Duration
	PostWindow  time.Duration
	SafetySlack time.Duration
}

type Highlighter struct {
	FFprobeBin  string
	Store       *store.StateStore
	Snapshotter *persist.Snapshotter
	Obs         *goobs.Client

	cfg Config

	mu sync.Mutex

	nextSessionID uint64
	sessions      []*bufferSession

	pendingMu sync.Mutex
	pending   []pendingSave

	saveCh chan uint64 // sessionID
	stopCh chan struct{}
}

func New(FFprobeBin string, store *store.StateStore, snapshotter *persist.Snapshotter, obsClient *goobs.Client) *Highlighter {
	hl := &Highlighter{
		FFprobeBin:  FFprobeBin,
		Store:       store,
		Snapshotter: snapshotter,
		Obs:         obsClient,
		cfg: Config{
			BufferLen:   20 * time.Second,
			PreWindow:   5 * time.Second,
			PostWindow:  5 * time.Second,
			SafetySlack: 250 * time.Millisecond,
		},
		saveCh: make(chan uint64, 64),
		stopCh: make(chan struct{}),
	}
	go hl.saveWorker()
	return hl
}

func (hl *Highlighter) getObs() *goobs.Client {
	return hl.Obs
}

func (hl *Highlighter) Close() {
	close(hl.stopCh)
}

func (hl *Highlighter) RecordHighlight() {
	now := time.Now()

	hl.mu.Lock()
	defer hl.mu.Unlock()

	var s *bufferSession
	if n := len(hl.sessions); n > 0 {
		last := hl.sessions[n-1]
		if !last.closed && !last.savedReq {
			s = last
		}
	}

	maxSpan := hl.cfg.BufferLen - hl.cfg.PreWindow - hl.cfg.PostWindow - hl.cfg.SafetySlack

	canAppend := func(session *bufferSession) bool {
		if session == nil {
			return false
		}

		if now.Sub(session.lastAt) > hl.cfg.PostWindow {
			return false
		}

		if now.Sub(session.firstAt) > maxSpan {
			return false
		}

		return true
	}

	if !canAppend(s) {
		hl.nextSessionID++
		s = &bufferSession{
			id:      hl.nextSessionID,
			firstAt: now,
			lastAt:  now,
			kills:   []time.Time{now},
		}

		s.timer = time.NewTimer(hl.cfg.PostWindow)
		go hl.sessionTimerLoop(s.id, s.timer)

		hl.sessions = append(hl.sessions, s)
		log.Printf("[Highlighter] new session=%d", s.id)
		return
	}

	s.kills = append(s.kills, now)
	s.lastAt = now

	if s.timer.Stop() {
		s.timer.Reset(hl.cfg.PostWindow)
	}
}

func (hl *Highlighter) sessionTimerLoop(sessionID uint64, t *time.Timer) {
	select {
	case <-t.C:
		hl.enqueueSave(sessionID)
	case <-hl.stopCh:
		return
	}
}

func (hl *Highlighter) enqueueSave(sessionID uint64) {
	hl.mu.Lock()
	defer hl.mu.Unlock()

	for i := len(hl.sessions) - 1; i >= 0; i-- {
		s := hl.sessions[i]
		if s.id != sessionID {
			continue
		}
		if s.closed || s.savedReq {
			return
		}
		s.closed = true
		hl.saveCh <- sessionID
		return
	}
}

func (hl *Highlighter) saveWorker() {
	for {
		select {
		case <-hl.stopCh:
			return
		case sessionID := <-hl.saveCh:
			if err := hl.saveReplayBufferForSession(sessionID); err != nil {
				log.Printf("[Highlighter] SaveReplayBuffer failed session=%d err=%v", sessionID, err)
			}
		}
	}
}

func (hl *Highlighter) saveReplayBufferForSession(sessionID uint64) error {
	obs := hl.getObs()
	if obs == nil {
		return errors.New("obs client is nil")
	}

	hl.mu.Lock()
	var kills []time.Time
	for i := range hl.sessions {
		if hl.sessions[i].id == sessionID {
			s := hl.sessions[i]
			if s.savedReq {
				hl.mu.Unlock()
				return nil
			}
			s.savedReq = true
			kills = append([]time.Time(nil), s.kills...)
			break
		}
	}
	hl.mu.Unlock()

	if len(kills) == 0 {
		return nil
	}

	status, err := obs.Outputs.GetReplayBufferStatus()
	if err == nil && !status.OutputActive {
		if _, err2 := obs.Outputs.StartReplayBuffer(); err2 != nil {
			return err2
		}
	}

	requestedAt := time.Now()

	if _, err = obs.Outputs.SaveReplayBuffer(); err != nil {
		return err
	}

	bufferStart := requestedAt.Add(-hl.cfg.BufferLen)

	offsets := make([]uint64, 0, len(kills))
	for _, kt := range kills {
		d := kt.Sub(bufferStart)
		if d < 0 {
			d = 0
		}
		if d > hl.cfg.BufferLen {
			d = hl.cfg.BufferLen
		}
		offsets = append(offsets, uint64(d.Milliseconds()))
	}

	ps := pendingSave{
		sessionID:    sessionID,
		requestedAt:  requestedAt,
		bufferStart:  bufferStart,
		eventsOffset: offsets,
	}

	hl.pendingMu.Lock()
	hl.pending = append(hl.pending, ps)
	hl.pendingMu.Unlock()

	log.Printf("[Highlighter] SaveReplayBuffer requested session=%d", sessionID)
	return nil
}

func (hl *Highlighter) OnReplayBufferSaved(savedReplayPath string) {
	hl.pendingMu.Lock()
	if len(hl.pending) == 0 {
		hl.pendingMu.Unlock()
		log.Printf("[Highlighter] ReplayBufferSaved but no pending saves; path=%s", savedReplayPath)
		return
	}
	ps := hl.pending[0]
	hl.pending = hl.pending[1:]
	hl.pendingMu.Unlock()

	state := hl.Store.Get()

	rbDuration, err := hl.ProbeDurationMs(context.Background(), savedReplayPath)

	if err != nil {
		rbDuration = uint64(hl.cfg.BufferLen.Milliseconds())
	}

	h := Highlight{
		MatchId:          state.MatchInfo.MatchID,
		StartTime:        uint64(ps.requestedAt.UnixMilli()) - rbDuration,
		MediaPath:        savedReplayPath,
		Duration:         rbDuration,
		EventsTimestamps: ps.eventsOffset,
		Round:            0,
	}

	for _, round := range state.MatchInfo.Rounds {
		if round.StartedAt.UnixMilli() >= int64(h.StartTime) && (round.EndedAt.UnixMilli() > int64(h.StartTime)) {
			h.Round = uint64(round.Number)
			break
		}
	}

	hl.Store.Update(func(cur domain.State) domain.State {
		next := cur
		next.ReplayState.PendingHighlights = append(cur.ReplayState.PendingHighlights, &h)

		return next
	})

	hl.Snapshotter.RequestSave()

	log.Printf("[Highlighter] replay saved session=%d path=%s offsets=%v", ps.sessionID, savedReplayPath, ps.eventsOffset)
}

func (hl *Highlighter) ProbeDurationMs(ctx context.Context, filePath string) (uint64, error) {
	if strings.TrimSpace(filePath) == "" {
		return 0, errors.New("filePath is empty")
	}

	cmd := exec.CommandContext(
		ctx,
		hl.FFprobeBin,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	stringDuration := strings.TrimSpace(string(out))
	if stringDuration == "" || stringDuration == "N/A" {
		return 0, errors.New("duration is not available")
	}

	sec, err := strconv.ParseFloat(stringDuration, 64)
	if err != nil {
		return 0, err
	}

	if sec < 0 {
		return 0, errors.New("duration is negative")
	}

	ms := uint64(math.Round(sec * 1000.0))
	return ms, nil
}
