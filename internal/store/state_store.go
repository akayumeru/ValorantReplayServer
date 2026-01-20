package store

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/akayumeru/valreplayserver/internal/domain"
)

type StateStore struct {
	mu      sync.Mutex
	current atomic.Value
	version atomic.Uint64
}

func NewStateStore(initial domain.State) *StateStore {
	s := &StateStore{}
	s.current.Store(initial)

	return s
}

func (s *StateStore) Get() domain.State {
	return s.current.Load().(domain.State)
}

func (s *StateStore) Version() uint64 {
	return s.version.Load()
}

func (s *StateStore) Update(fn func(cur domain.State) domain.State) domain.State {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.Get()
	next := fn(cur)
	next.UpdatedAt = time.Now().UTC()

	s.current.Store(next)
	s.version.Add(1)

	return next
}

func (s *StateStore) Replace(next domain.State) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next.UpdatedAt = time.Now().UTC()
	s.current.Store(next)
	s.version.Add(1)
}
