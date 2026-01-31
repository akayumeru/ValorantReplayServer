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
	events   []time.Time
	timer    *time.Timer
	closed   bool
	savedReq bool

	waitCh chan error
}

type pendingSave struct {
	sessionID   uint64
	requestedAt time.Time
	bufferStart time.Time   // requestedAt - bufferLen
	events      []time.Time // offsets from bufferStart in ms

	waitCh chan error
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

func (hl *Highlighter) FlushIfHasHighlightsNow(ctx context.Context) (bool, error) {
	hl.mu.Lock()

	var target *bufferSession
	for i := len(hl.sessions) - 1; i >= 0; i-- {
		s := hl.sessions[i]
		if len(s.events) == 0 {
			continue
		}
		if s.savedReq && s.waitCh != nil {
			ch := s.waitCh
			hl.mu.Unlock()

			select {
			case err := <-ch:
				return true, err
			case <-ctx.Done():
				return true, ctx.Err()
			}
		}

		if !s.savedReq {
			target = s
			break
		}
	}

	if target == nil {
		hl.mu.Unlock()
		return false, nil
	}

	target.closed = true
	if target.timer != nil {
		_ = target.timer.Stop()
	}

	sessionID := target.id
	hl.mu.Unlock()

	return true, hl.SaveReplayBufferForSessionAndWait(ctx, sessionID)
}

func (hl *Highlighter) SaveReplayBufferForSession(sessionID uint64) error {
	_, err := hl.requestSave(sessionID, nil)
	return err
}

func (hl *Highlighter) SaveReplayBufferForSessionAndWait(ctx context.Context, sessionID uint64) error {
	waitCh, err := hl.requestSave(sessionID, func() chan error { return make(chan error, 1) })
	if err != nil {
		if waitCh == nil {
			return err
		}
	}

	if waitCh == nil {
		return nil
	}

	select {
	case e := <-waitCh:
		return e
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (hl *Highlighter) requestSave(sessionID uint64, makeWaitCh func() chan error) (chan error, error) {
	obs := hl.getObs()
	if obs == nil {
		return nil, errors.New("obs client is nil")
	}

	hl.mu.Lock()

	var s *bufferSession
	for i := range hl.sessions {
		if hl.sessions[i].id == sessionID {
			s = hl.sessions[i]
			break
		}
	}
	if s == nil {
		hl.mu.Unlock()
		return nil, nil
	}

	if len(s.events) == 0 {
		hl.mu.Unlock()
		return nil, nil
	}

	if s.savedReq {
		ch := s.waitCh
		hl.mu.Unlock()
		return ch, nil
	}

	s.closed = true
	if s.timer != nil {
		_ = s.timer.Stop()
	}

	var ch chan error
	if makeWaitCh != nil {
		ch = makeWaitCh()
		s.waitCh = ch
	}

	s.savedReq = true

	events := append([]time.Time(nil), s.events...)
	hl.mu.Unlock()

	status, err := obs.Outputs.GetReplayBufferStatus()
	if err == nil && !status.OutputActive {
		if _, err2 := obs.Outputs.StartReplayBuffer(); err2 != nil {
			hl.failSave(sessionID, ch, err2)
			return ch, err2
		}
	}

	requestedAt := time.Now()
	if _, err := obs.Outputs.SaveReplayBuffer(); err != nil {
		hl.failSave(sessionID, ch, err)
		return ch, err
	}

	ps := pendingSave{
		sessionID:   sessionID,
		requestedAt: requestedAt,
		events:      events,
		waitCh:      ch,
	}

	hl.pendingMu.Lock()
	hl.pending = append(hl.pending, ps)
	hl.pendingMu.Unlock()

	log.Printf("[Highlighter] SaveReplayBuffer requested session=%d", sessionID)
	return ch, nil
}

func (hl *Highlighter) failSave(sessionID uint64, ch chan error, err error) {
	hl.mu.Lock()
	for i := range hl.sessions {
		if hl.sessions[i].id == sessionID {
			hl.sessions[i].savedReq = false
			hl.sessions[i].closed = false
			hl.sessions[i].waitCh = nil
			break
		}
	}
	hl.mu.Unlock()

	if ch != nil {
		ch <- err
		close(ch)
	}
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
			events:  []time.Time{now},
		}

		s.timer = time.NewTimer(hl.cfg.PostWindow)
		go hl.sessionTimerLoop(s.id, s.timer)

		hl.sessions = append(hl.sessions, s)
		log.Printf("[Highlighter] new session=%d", s.id)
		return
	}

	s.events = append(s.events, now)
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
			if _, err := hl.requestSave(sessionID, nil); err != nil {
				log.Printf("[Highlighter] SaveReplayBuffer failed session=%d err=%v", sessionID, err)
			}
		}
	}
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

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	rbDuration, err := hl.ProbeDurationMs(ctx, savedReplayPath)
	if err != nil || rbDuration == 0 {
		rbDuration = uint64(hl.cfg.BufferLen.Milliseconds())
	}

	bufferStart := ps.requestedAt.Add(-time.Duration(rbDuration) * time.Millisecond)
	duration := time.Duration(rbDuration) * time.Millisecond

	offsets := make([]uint64, 0, len(ps.events))
	for _, kt := range ps.events {
		d := kt.Sub(bufferStart)
		if d < 0 {
			d = 0
		}
		if d > duration {
			d = duration
		}
		offsets = append(offsets, uint64(d.Milliseconds()))
	}

	h := Highlight{
		MatchId:          state.MatchInfo.MatchID,
		StartTime:        uint64(bufferStart.UnixMilli()),
		MediaPath:        savedReplayPath,
		Duration:         rbDuration,
		EventsTimestamps: offsets,
		Round:            0,
	}

	for _, round := range state.MatchInfo.Rounds {
		if round.StartedAt.UnixMilli() <= int64(h.StartTime) && (round.EndedAt.IsZero() || round.EndedAt.UnixMilli() > int64(h.StartTime)) {
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

	if ps.waitCh != nil {
		ps.waitCh <- nil
		close(ps.waitCh)
	}

	hl.mu.Lock()
	for i := range hl.sessions {
		if hl.sessions[i].id == ps.sessionID {
			hl.sessions[i].waitCh = nil
			break
		}
	}
	hl.mu.Unlock()

	log.Printf("[Highlighter] replay saved session=%d path=%s offsets=%v", ps.sessionID, savedReplayPath, offsets)
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
