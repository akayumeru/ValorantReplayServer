package persist

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
	"github.com/akayumeru/valreplayserver/internal/store"
	"github.com/sashka/atomicfile"
)

type Snapshotter struct {
	path           string
	store          *store.StateStore
	debounceWindow time.Duration

	reqCh chan struct{}
}

func NewSnapshotter(path string, st *store.StateStore, debounceWindow time.Duration) *Snapshotter {
	return &Snapshotter{
		path:           path,
		store:          st,
		debounceWindow: debounceWindow,
		reqCh:          make(chan struct{}, 1),
	}
}

func (s *Snapshotter) RequestSave() {
	select {
	case s.reqCh <- struct{}{}:
	default:
	}
}

func (s *Snapshotter) LoadOnStartup() (domain.State, bool, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.State{}, false, nil
		}
		return domain.State{}, false, err
	}

	var st domain.State
	if err := json.Unmarshal(b, &st); err != nil {
		return domain.State{}, false, err
	}
	return st, true, nil
}

func (s *Snapshotter) Run(ctx context.Context) error {
	var (
		timer   *time.Timer
		timerCh <-chan time.Time
	)

	stopTimer := func() {
		if timer != nil {
			timer.Stop()
		}
		timer = nil
		timerCh = nil
	}

	for {
		select {
		case <-ctx.Done():
			_ = s.writeOnce()
			return nil

		case <-s.reqCh:
			if timer == nil {
				timer = time.NewTimer(s.debounceWindow)
				timerCh = timer.C
			} else {
				// reset debounce
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(s.debounceWindow)
			}

		case <-timerCh:
			stopTimer()
			if err := s.writeOnce(); err != nil {
			}
		}
	}
}

func (s *Snapshotter) writeOnce() error {
	state := s.store.Get()

	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}

	f, err := atomicfile.New(s.path, 0o666)
	defer f.Abort()

	_, err = f.Write(payload)

	if err != nil {
		log.Printf("Failed to write state to %q: %v\n", s.path, err)
	} else {
		log.Printf("Wrote state to %q\n", s.path)
		f.Close()
	}

	return err
}
