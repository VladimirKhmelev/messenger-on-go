package events

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func TestPoolChatOrderAndParallelism(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPool(t.Context(), Config{Workers: 2, QueueSize: 8, Timeout: time.Minute})
		defer p.close()
		release := make(chan struct{})
		var got []int // only the worker for chat A writes this slice
		if err := p.submit("A", func(context.Context) error { <-release; got = append(got, 1); return nil }); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		for _, n := range []int{2, 3} {
			if err := p.submit("A", func(context.Context) error { got = append(got, n); return nil }); err != nil {
				t.Fatal(err)
			}
		}
		other := make(chan struct{})
		if err := p.submit("B", func(context.Context) error { close(other); return nil }); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case <-other:
		default:
			t.Fatal("chat B blocked behind chat A")
		}
		if len(got) != 0 {
			t.Fatalf("same-chat handlers overlapped: %v", got)
		}
		close(release)
		synctest.Wait()
		if !slices.Equal(got, []int{1, 2, 3}) {
			t.Fatalf("order = %v", got)
		}
		if len(p.lanes) != 0 || p.pending != 0 {
			t.Fatal("completed keys retained")
		}
	})
}

func TestPoolFairness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPool(t.Context(), Config{Workers: 1, QueueSize: 8, Timeout: time.Minute})
		defer p.close()
		release := make(chan struct{})
		var got []string
		if err := p.submit("A", func(context.Context) error { <-release; return nil }); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		for _, key := range []string{"A", "A", "B"} {
			if err := p.submit(key, func(context.Context) error { got = append(got, key); return nil }); err != nil {
				t.Fatal(err)
			}
		}
		close(release)
		synctest.Wait()
		if !slices.Equal(got, []string{"B", "A", "A"}) {
			t.Fatalf("ready keys did not take turns: %v", got)
		}
	})
}

func TestPoolBackpressureAndShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPool(t.Context(), Config{Workers: 1, QueueSize: 1, Timeout: time.Minute})
		if err := p.submit("A", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		ran := false
		if err := p.submit("B", func(context.Context) error { ran = true; return nil }); err != nil {
			t.Fatal(err)
		}
		submitted := make(chan error, 1)
		go func() { submitted <- p.submit("C", func(context.Context) error { ran = true; return nil }) }()
		synctest.Wait()
		select {
		case err := <-submitted:
			t.Fatalf("full queue accepted another job: %v", err)
		default:
		}
		if p.pending != 1 {
			t.Fatalf("queued = %d", p.pending)
		}
		p.close()
		if err := <-submitted; !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked submit = %v", err)
		}
		if ran || len(p.lanes) != 0 || len(p.ready) != 0 {
			t.Fatal("shutdown executed or retained queued work")
		}
	})
}

func TestPoolTimeoutStartsAfterQueueAndErrorsDoNotStopLane(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPool(t.Context(), Config{Workers: 1, QueueSize: 3, Timeout: 10 * time.Second})
		defer p.close()
		start := time.Now()
		if err := p.submit("A", func(context.Context) error { time.Sleep(7 * time.Second); return errors.New("RPC failed") }); err != nil {
			t.Fatal(err)
		}
		var deadline time.Time
		if err := p.submit("A", func(ctx context.Context) error { deadline, _ = ctx.Deadline(); <-ctx.Done(); return ctx.Err() }); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		if err := p.submit("A", func(context.Context) error { close(done); return nil }); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Second)
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("timeout included queue wait")
		default:
		}
		if !deadline.Equal(start.Add(17 * time.Second)) {
			t.Fatalf("deadline = %v", deadline.Sub(start))
		}
		time.Sleep(7 * time.Second)
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("lane stopped after timeout")
		}
	})
}

func TestConfigValidation(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []Config{{}, {0, 1, time.Second}, {1, 0, time.Second}, {1, 1, 0}, {1, 1, -1}, {1, 1, time.Duration(1<<63 - 1)}} {
		if err := cfg.Validate(); err == nil {
			t.Errorf("accepted %+v", cfg)
		}
	}
}
