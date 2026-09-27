package hub

import (
	"sync"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
)

type Registry struct {
	mu      sync.RWMutex
	clients map[string]map[Client]struct{}
}

func NewRegistry() *Registry {
	return &Registry{clients: make(map[string]map[Client]struct{})}
}

func (r *Registry) Add(userID string, c Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.clients[userID] == nil {
		r.clients[userID] = make(map[Client]struct{})
	}
	r.clients[userID][c] = struct{}{}
	metrics.WSConnectionOpened()
}

func (r *Registry) Remove(userID string, c Client) (hasOtherClients bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients[userID], c)
	metrics.WSConnectionClosed()
	if len(r.clients[userID]) == 0 {
		delete(r.clients, userID)
		return false
	}
	return true
}

func (r *Registry) Broadcast(userID string, notification any) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for c := range r.clients[userID] {
		c.Deliver(notification)
	}
}
