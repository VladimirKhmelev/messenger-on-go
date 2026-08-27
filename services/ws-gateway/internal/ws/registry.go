package ws

import (
	"sync"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
)

type Registry struct {
	mu       sync.RWMutex
	sessions map[string]map[*session]struct{}
}

func NewRegistry() *Registry {
	return &Registry{sessions: make(map[string]map[*session]struct{})}
}

func (r *Registry) add(s *session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.sessions[s.userID] == nil {
		r.sessions[s.userID] = make(map[*session]struct{})
	}
	r.sessions[s.userID][s] = struct{}{}
	metrics.WSConnectionOpened()
}

func (r *Registry) remove(s *session) (hasOtherSessions bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessions[s.userID], s)
	metrics.WSConnectionClosed()
	if len(r.sessions[s.userID]) == 0 {
		delete(r.sessions, s.userID)
		return false
	}
	return true
}

func (r *Registry) Broadcast(userID string, payload serverMessage) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for s := range r.sessions[userID] {
		s.write(payload)
	}
}
