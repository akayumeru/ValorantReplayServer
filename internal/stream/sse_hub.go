package stream

import "sync"

type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan []byte]struct{})}
}

func (h *Hub) Subscribe(topic string) (ch chan []byte, cancel func()) {
	ch = make(chan []byte)

	h.mu.Lock()
	if h.subs[topic] == nil {
		h.subs[topic] = make(map[chan []byte]struct{})
	}
	h.subs[topic][ch] = struct{}{}
	h.mu.Unlock()

	cancel = func() {
		h.mu.Lock()
		delete(h.subs[topic], ch)
		h.mu.Unlock()
		close(ch)
	}

	return ch, cancel
}

func (h *Hub) Publish(topic string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.subs[topic] {
		select {
		case ch <- payload:
		default:
		}
	}
}
