package main

import "time"

// delivery belongs to one actual send on one connection. The opaque text
// token correlates notifications even when they precede message_sent.
// All fields are protected by conn.mu while readers are running.
type delivery struct {
	step       int
	due        time.Time
	order      uint64
	accepted   bool
	ackID      string
	received   bool
	id         string
	latency    time.Duration
	duplicates int
	reordered  bool
	mismatch   bool
}

func (c *conn) received(m serverMessage, now time.Time) {
	if m.Message == nil || m.ChatID != c.user.chatID || m.Message.MessageID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.deliveries[m.Message.Text]
	if d == nil {
		return
	}
	if d.received {
		d.duplicates++
		if d.id != m.Message.MessageID {
			d.mismatch = true
		}
		return
	}
	d.received = true
	d.id = m.Message.MessageID
	d.latency = now.Sub(d.due)
	d.reordered = d.order < c.lastDelivered
	c.lastDelivered = max(c.lastDelivered, d.order)
	d.mismatch = d.accepted && d.ackID != d.id
}

type deliveryTally struct {
	attempted, accepted, received, missing, unconfirmed int
	duplicates, reordered, mismatches                   int
	latencies                                           []time.Duration
}

func (r *recorder) delivery(g int, d *delivery) {
	if d.step < 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key{step: d.step, gateway: g, kind: sendMessage}
	t := r.deliveries[k]
	if t == nil {
		t = &deliveryTally{}
		r.deliveries[k] = t
	}
	t.attempted++
	if d.accepted {
		t.accepted++
		if d.received && d.id == d.ackID {
			t.received++
			t.latencies = append(t.latencies, d.latency)
		} else {
			t.missing++
		}
	} else if d.received {
		t.unconfirmed++
	}
	t.duplicates += d.duplicates
	if d.reordered {
		t.reordered++
	}
	if d.mismatch {
		t.mismatches++
	}
}
