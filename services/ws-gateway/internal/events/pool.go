package events

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	vmetrics "github.com/VictoriaMetrics/metrics"
)

// Config bounds concurrent fanout work, queued events and processing time.
// Timeout starts when a worker starts the event, excluding time in the queue.
type Config struct {
	Workers   int
	QueueSize int
	Timeout   time.Duration
}

func DefaultConfig() Config { return Config{Workers: 16, QueueSize: 256, Timeout: 10 * time.Second} }

func (c Config) Validate() error {
	if c.Workers < 1 || c.QueueSize < 1 || c.Timeout <= 0 || c.Timeout > time.Duration(1<<63-1)/(pullBatch+2) {
		return errors.New("fanout: workers, queue size and timeout must be positive; timeout is too large if consumer lifetime overflows")
	}
	return nil
}

type job struct {
	queued time.Time
	run    func(context.Context) error
}

type lane struct {
	key    string
	jobs   []job
	active bool
}

// pool schedules one event of a key at a time. Ready keys take turns, so a
// busy chat neither occupies extra workers nor starves another ready chat.
type pool struct {
	ctx     context.Context
	cancel  context.CancelFunc
	cfg     Config
	mu      sync.Mutex
	changed chan struct{}
	lanes   map[string]*lane
	ready   []*lane
	pending int
	workers sync.WaitGroup
}

func newPool(ctx context.Context, cfg Config) *pool {
	ctx, cancel := context.WithCancel(ctx)
	p := &pool{ctx: ctx, cancel: cancel, cfg: cfg, changed: make(chan struct{}), lanes: make(map[string]*lane)}
	for range cfg.Workers {
		p.workers.Go(p.work)
	}
	return p
}

func (p *pool) signal() { close(p.changed); p.changed = make(chan struct{}) }

// submit applies backpressure rather than discarding queued events. The
// caller must submit a given key in source order; this method preserves it.
func (p *pool) submit(key string, run func(context.Context) error) error {
	for {
		p.mu.Lock()
		if err := p.ctx.Err(); err != nil {
			p.mu.Unlock()
			return err
		}
		if p.pending < p.cfg.QueueSize {
			l := p.lanes[key]
			if l == nil {
				l = &lane{key: key}
				p.lanes[key] = l
			}
			if !l.active && len(l.jobs) == 0 {
				p.ready = append(p.ready, l)
			}
			l.jobs = append(l.jobs, job{queued: time.Now(), run: run})
			p.pending++
			p.signal()
			p.mu.Unlock()
			return nil
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		case <-changed:
		}
	}
}

func (p *pool) work() {
	for {
		p.mu.Lock()
		if p.ctx.Err() != nil {
			p.mu.Unlock()
			return
		}
		if len(p.ready) == 0 {
			changed := p.changed
			p.mu.Unlock()
			select {
			case <-p.ctx.Done():
				return
			case <-changed:
			}
			continue
		}
		l := p.ready[0]
		p.ready[0] = nil
		p.ready = p.ready[1:]
		j := l.jobs[0]
		l.jobs[0] = job{}
		l.jobs = l.jobs[1:]
		l.active = true
		p.pending--
		p.signal()
		p.mu.Unlock()

		started := time.Now()
		vmetrics.GetOrCreateHistogram("ws_fanout_queue_wait_seconds").UpdateDuration(j.queued)
		ctx, cancel := context.WithTimeout(p.ctx, p.cfg.Timeout)
		err := j.run(ctx)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		cancel()
		vmetrics.GetOrCreateHistogram("ws_fanout_processing_seconds").UpdateDuration(started)
		outcome := "ok"
		if err != nil {
			outcome = "error"
			if errors.Is(err, context.DeadlineExceeded) {
				outcome = "timeout"
			}
			if errors.Is(err, context.Canceled) {
				outcome = "canceled"
			}
			log.Printf("ws-gateway: fanout %s: %v", l.key, err)
		}
		vmetrics.GetOrCreateCounter(`ws_fanout_events_total{outcome="` + outcome + `"}`).Inc()

		p.mu.Lock()
		l.active = false
		if len(l.jobs) > 0 {
			p.ready = append(p.ready, l)
		} else {
			delete(p.lanes, l.key)
		}
		p.signal()
		p.mu.Unlock()
	}
}

// close cancels running work and releases queued events without starting
// them. This gateway uses best-effort notifications, not durable delivery.
func (p *pool) close() {
	p.cancel()
	p.workers.Wait()
	p.mu.Lock()
	clear(p.lanes)
	p.ready = nil
	p.pending = 0
	p.mu.Unlock()
}
