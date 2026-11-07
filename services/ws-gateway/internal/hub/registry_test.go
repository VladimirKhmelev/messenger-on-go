package hub

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
)

type deliveryClient struct{ deliver func(context.Context) error }

func (c *deliveryClient) Deliver(ctx context.Context, _ any) error { return c.deliver(ctx) }

func TestRegistrySlowClientDoesNotHoldLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := NewRegistry()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		started := make(chan struct{})
		slow := &deliveryClient{deliver: func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() }}
		r.Add("slow", slow)
		done := make(chan error, 1)
		go func() { done <- r.Broadcast(ctx, "slow", nil) }()
		<-started
		fast := &deliveryClient{deliver: func(context.Context) error { return nil }}
		other := make(chan error, 1)
		go func() {
			r.Add("fast", fast)
			err := r.Broadcast(t.Context(), "fast", nil)
			r.Remove("fast", fast)
			r.Remove("slow", slow)
			other <- err
		}()
		synctest.Wait()
		select {
		case err := <-other:
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("registry blocked on network I/O")
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}

func TestRegistryContinuesAfterClientError(t *testing.T) {
	r := NewRegistry()
	failure := errors.New("closed connection")
	r.Add("u", &deliveryClient{deliver: func(context.Context) error { return failure }})
	called := false
	r.Add("u", &deliveryClient{deliver: func(context.Context) error { called = true; return nil }})
	if err := r.Broadcast(t.Context(), "u", nil); !errors.Is(err, failure) || !called {
		t.Fatalf("error = %v; delivered to healthy session = %v", err, called)
	}
}
