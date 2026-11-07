package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func notification(t *testing.T, chat, token, id string) serverMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{"type": "message_received", "chat_id": chat, "message": map[string]string{"text": token, "message_id": id}})
	if err != nil {
		t.Fatal(err)
	}
	var m serverMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDeliveryIdentityOrderAndDuplicates(t *testing.T) {
	due := time.Now()
	a := &delivery{step: 0, due: due, order: 1, accepted: true, ackID: "a"}
	b := &delivery{step: 0, due: due, order: 2, accepted: true, ackID: "b"}
	c := &conn{user: user{chatID: "chat"}, deliveries: map[string]*delivery{"t1": a, "t2": b}}
	// Foreign chats, unknown tokens and incomplete notifications are ignored.
	for _, m := range []serverMessage{notification(t, "other", "t1", "a"), notification(t, "chat", "other", "a"), notification(t, "chat", "t1", ""), {Type: "message_received"}} {
		c.received(m, due)
	}
	if a.received || b.received {
		t.Fatal("unrelated frame counted")
	}
	c.received(notification(t, "chat", "t2", "b"), due.Add(30*time.Millisecond))
	c.received(notification(t, "chat", "t1", "a"), due.Add(40*time.Millisecond))
	c.received(notification(t, "chat", "t1", "a"), due.Add(time.Second))
	c.received(notification(t, "chat", "t1", "wrong"), due.Add(time.Second))
	if !a.reordered || b.reordered || a.duplicates != 2 || !a.mismatch || a.latency != 40*time.Millisecond {
		t.Fatalf("first = %+v; second = %+v", a, b)
	}
	r := newRecorder()
	r.delivery(0, a)
	r.delivery(0, b)
	got := r.deliveries[key{0, 0, sendMessage}]
	if got.received != 2 || got.missing != 0 || got.duplicates != 2 || got.reordered != 1 || got.mismatches != 1 || len(got.latencies) != 2 {
		t.Fatalf("tally = %+v", got)
	}
}

func TestDeliveryDenominators(t *testing.T) {
	r := newRecorder()
	for _, d := range []delivery{
		{step: -1, accepted: true, received: true, ackID: "warmup", id: "warmup"},
		{accepted: true, received: true, ackID: "a", id: "a", latency: 5 * time.Millisecond},
		{accepted: true, ackID: "b"}, // accepted but missing notification
		{received: true, id: "c"},    // notification without an accepted ACK
		{},                           // failed or missing ACK, no notification
		{accepted: true, received: true, ackID: "d", id: "wrong", mismatch: true},
	} {
		r.delivery(0, &d)
	}
	got := r.deliveries[key{0, 0, sendMessage}]
	if got.attempted != 5 || got.accepted != 3 || got.received != 1 || got.missing != 2 || got.unconfirmed != 1 || got.mismatches != 1 || len(got.latencies) != 1 {
		t.Fatalf("tally = %+v", got)
	}
	c := config{gateways: []gateway{{"quic", ""}}, rates: []float64{10}, step: time.Second}
	var b bytes.Buffer
	r.report(c, time.Now()).print(&b)
	for _, want := range []string{"message delivery", "accepted", "out_of_order", "id_mismatch", "5.00"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no %q in %s", want, b.String())
		}
	}
}

func TestRunObservesDeliveryBeforeAck(t *testing.T) {
	s := newStack(t)
	c := s.start(80)
	c.mix = mix{sendMessage: 1}
	c.drain = 50 * time.Millisecond
	rep, err := run(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	for g := range c.gateways {
		d := rep.deliveries[key{0, g, sendMessage}]
		ack := rep.tallies[key{0, g, sendMessage}]
		if d == nil || d.accepted == 0 || d.accepted != ack.ok || d.received != d.accepted || d.missing != 0 || d.mismatches != 0 || d.reordered != 0 || d.duplicates != 0 {
			t.Fatalf("gateway %d: deliveries %+v, ACKs %+v", g, d, ack)
		}
	}
}

func TestRunObservesLateDuplicateAndMissingDeliveries(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "late_duplicates", true: "missing"}[missing], func(t *testing.T) {
			s := newStack(t)
			s.deliveryDelay, s.duplicates, s.dropDelivery = 60*time.Millisecond, 1, missing
			c := s.start(120)
			c.mix, c.warmup, c.step, c.drain = mix{sendMessage: 1}, 0, 150*time.Millisecond, time.Second
			rep, err := run(t.Context(), c)
			if err != nil {
				t.Fatal(err)
			}
			for g := range c.gateways {
				d := rep.deliveries[key{0, g, sendMessage}]
				if d == nil || d.accepted == 0 || d.accepted != d.attempted {
					t.Fatalf("gateway %d: %+v", g, d)
				}
				if missing {
					if d.missing != d.accepted || d.received != 0 || len(d.latencies) != 0 {
						t.Fatalf("missing delivery = %+v", d)
					}
				} else {
					if d.received != d.accepted || d.missing != 0 || d.duplicates != d.accepted {
						t.Fatalf("late delivery = %+v", d)
					}
					for _, latency := range d.latencies {
						if latency < s.deliveryDelay {
							t.Fatalf("latency %v excludes delayed delivery", latency)
						}
					}
				}
			}
		})
	}
}
