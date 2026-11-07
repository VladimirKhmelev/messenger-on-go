package hub

import (
	"context"
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

// Broadcast snapshots clients under the registry lock, then performs I/O
// without holding it. An in-flight delivery may finish after Remove.
func (r *Registry) Broadcast(ctx context.Context, userID string, notification any) error {
	r.mu.RLock()
	clients := make([]Client, 0, len(r.clients[userID]))
	for c := range r.clients[userID] {
		clients = append(clients, c)
	}
	r.mu.RUnlock()
	var first error
	for _, c := range clients {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.Deliver(ctx, notification); err != nil && first == nil {
			first = err
		}
	}
	return first
}
