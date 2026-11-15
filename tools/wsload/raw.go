package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"
)

// All offsets use the monotonic clock of one load process, in nanoseconds.
type callRecord struct {
	ResponseOutcome, ResponseError             string
	Sent, ResponseSeen                         bool
	ID, Kind, User, Peer, Chat, Outcome, Error string
	Step, Gateway                              int
	ScheduledNS, DeadlineNS, SendNS            int64
	ResponseNS, CompleteNS, Bytes              int64
	release                                    func()
	timer                                      *time.Timer
}

type receipt struct {
	Received, Notified                     bool
	Call, Receiver, Token, Chat, AckID, ID string
	Gateway, Step                          int
	Order                                  uint64
	ScheduledNS, ReceivedNS, NotifyNS      int64
	Accepted, Reordered, Mismatch          bool
	Duplicates, NotifyDuplicates           int
}

type rawEvent struct {
	Kind, User, Chat, MessageID, OperationID string
	AtNS                                     int64
}

type rawRecords struct {
	events        []rawEvent
	id            string
	mu            sync.Mutex
	start         time.Time
	calls         []*callRecord
	receipts      map[string]*receipt
	notifies      map[string]*receipt
	last          map[string]uint64
	notifications []notifyObservation
}

func newRaw(start time.Time) *rawRecords {
	return &rawRecords{id: newID(), start: start, receipts: map[string]*receipt{}, notifies: map[string]*receipt{}, last: map[string]uint64{}}
}

func (r *rawRecords) plan(g, n int, a arrival, u user, timeout time.Duration) *callRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	x := &callRecord{ID: fmt.Sprintf("g%d-%d", g, n), Kind: kindNames[a.kind], User: u.id, Peer: u.peerID, Chat: u.chatID,
		Step: a.step, Gateway: g, ScheduledNS: int64(a.at), DeadlineNS: int64(a.at + timeout), Outcome: "planned"}
	r.calls = append(r.calls, x)
	return x
}

func (r *rawRecords) send(x *callRecord, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if x.Outcome != "planned" {
		return false
	}
	x.SendNS, x.Outcome, x.Sent = int64(now.Sub(r.start)), "pending", true
	return true
}

func (r *rawRecords) finish(x *callRecord, now time.Time, outcome, failure string, bytes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if outcome == "success" || outcome == "rpc_error" {
		x.ResponseNS, x.ResponseSeen = max(0, int64(now.Sub(r.start))), true
		x.ResponseOutcome, x.ResponseError = outcome, failure
	}
	if x.Outcome != "planned" && x.Outcome != "pending" {
		return
	}
	releaseCall(x)
	x.CompleteNS = max(0, int64(now.Sub(r.start)))
	if outcome == "success" || outcome == "rpc_error" {
		x.ResponseNS = x.CompleteNS
	}
	if x.CompleteNS > x.DeadlineNS && x.Sent {
		x.Outcome, x.CompleteNS, x.Error = "deadline", x.DeadlineNS, "deadline_exceeded"
	} else {
		x.Outcome, x.Error = outcome, failure
	}
	if x.Outcome == "success" {
		x.Bytes = bytes
	}
}

func receiptKey(chat, token string) string { return chat + "\x00" + token }

func (r *rawRecords) expect(x *callRecord, token, receiver string, order uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.receipts[receiptKey(x.Chat, token)] = &receipt{Call: x.ID, Receiver: receiver, Token: token, Chat: x.Chat,
		Gateway: x.Gateway, Step: x.Step, Order: order, ScheduledNS: x.ScheduledNS}
}

func (r *rawRecords) ack(x *callRecord, token, id string, accepted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.receipts[receiptKey(x.Chat, token)]
	if d == nil {
		return
	}
	d.AckID, d.Accepted = id, accepted
	if d.Received && accepted && d.ID != id {
		d.Mismatch = true
	}
	if id != "" {
		r.notifies[d.Receiver+"\x00"+id] = d
	}
}

func (r *rawRecords) receive(userID string, m serverMessage, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	event := rawEvent{Kind: "message_received", User: userID, Chat: m.ChatID, AtNS: int64(now.Sub(r.start))}
	if m.Message != nil {
		event.MessageID = m.Message.MessageID
	}
	r.events = append(r.events, event)
	if m.Message == nil || m.Message.MessageID == "" {
		return
	}
	d := r.receipts[receiptKey(m.ChatID, m.Message.Text)]
	if d == nil || d.Receiver != userID {
		return
	}
	r.events[len(r.events)-1].OperationID = d.Call
	if d.Received {
		d.Duplicates++
		if d.ID != m.Message.MessageID {
			d.Mismatch = true
		}
		return
	}
	d.ID, d.ReceivedNS, d.Received = m.Message.MessageID, int64(now.Sub(r.start)), true
	orderKey := userID + "\x00" + m.ChatID + "\x00" + strconv.Itoa(d.Gateway)
	d.Reordered = d.Order < r.last[orderKey]
	r.last[orderKey] = max(r.last[orderKey], d.Order)
	d.Mismatch = d.Accepted && d.AckID != d.ID
	r.notifies[userID+"\x00"+d.ID] = d
}

// Notification may precede both ACK and message_received, so preserve it by ID.
type notifyObservation struct {
	User, ID string
	AtNS     int64
}

func (r *rawRecords) close(now time.Time, canceled bool, notifications []notifyObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	end := max(0, int64(now.Sub(r.start)))
	for _, x := range r.calls {
		releaseCall(x)
		switch x.Outcome {
		case "planned":
			x.Outcome, x.CompleteNS, x.Error = "not_sent", end, "run_canceled"
		case "pending":
			x.Outcome, x.CompleteNS, x.Error = "no_response", end, "observation_ended"
			if end >= x.DeadlineNS {
				x.Outcome, x.CompleteNS, x.Error = "deadline", x.DeadlineNS, "deadline_exceeded"
			}
			if canceled {
				x.Error = "run_canceled"
			}
		}
	}
	for i, event := range r.events {
		if event.Kind == "notify_push" {
			if d := r.notifies[event.User+"\x00"+event.MessageID]; d != nil {
				r.events[i].OperationID = d.Call
			}
		}
	}
	notifications = append(notifications, r.notifications...)
	for _, n := range notifications {
		d := r.notifies[n.User+"\x00"+n.ID]
		if d == nil {
			continue
		}
		if d.Notified {
			d.NotifyDuplicates++
		} else {
			d.NotifyNS, d.Notified = n.AtNS, true
		}
	}
}

func atomicJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(path+".tmp", append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func writeCSV(path string, header []string, rows [][]string) error {
	f, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	err = w.Write(header)
	for _, row := range rows {
		if err == nil {
			err = w.Write(row)
		}
	}
	w.Flush()
	if err == nil {
		err = w.Error()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func integer(v int64) string { return strconv.FormatInt(v, 10) }

func (r *rawRecords) save(dir string, c config, end time.Time, runErr error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var rows [][]string
	for _, x := range r.calls {
		rows = append(rows, []string{x.ID, integer(int64(x.Step)), integer(int64(x.Gateway)), x.Kind, x.User, x.Peer, x.Chat,
			integer(x.ScheduledNS), integer(x.DeadlineNS), optionalInteger(x.SendNS, x.Sent), optionalInteger(x.ResponseNS, x.ResponseSeen), integer(x.CompleteNS),
			x.Outcome, x.Error, x.ResponseOutcome, x.ResponseError, integer(max(0, x.CompleteNS-x.ScheduledNS)), integer(x.Bytes)})
	}
	if err := writeCSV(filepath.Join(dir, "calls.csv"), []string{"operation_id", "step", "gateway", "kind", "user_id", "peer_id", "chat_id", "scheduled_ns", "deadline_ns", "send_ns", "response_ns", "complete_ns", "outcome", "error", "response_outcome", "response_error", "latency_ns", "useful_bytes"}, rows); err != nil {
		return err
	}
	rows = nil
	keys := make([]string, 0, len(r.receipts))
	for key := range r.receipts {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		d := r.receipts[key]
		rows = append(rows, []string{d.Call, integer(int64(d.Step)), integer(int64(d.Gateway)), d.Receiver, d.Chat, d.AckID, d.ID,
			integer(d.ScheduledNS), optionalInteger(d.ReceivedNS, d.Received), optionalInteger(d.NotifyNS, d.Notified), strconv.FormatBool(d.Accepted), integer(int64(d.Duplicates)),
			integer(int64(d.NotifyDuplicates)), strconv.FormatBool(d.Reordered), strconv.FormatBool(d.Mismatch)})
	}
	if err := writeCSV(filepath.Join(dir, "deliveries.csv"), []string{"operation_id", "step", "sender_gateway", "receiver_id", "chat_id", "ack_message_id", "received_message_id", "scheduled_ns", "received_ns", "notify_ns", "accepted", "duplicates", "notify_duplicates", "out_of_order", "id_mismatch"}, rows); err != nil {
		return err
	}
	rows = nil
	for _, event := range r.events {
		rows = append(rows, []string{event.Kind, event.User, event.Chat, event.MessageID, event.OperationID, integer(event.AtNS)})
	}
	if err := writeCSV(filepath.Join(dir, "events.csv"), []string{"kind", "user_id", "chat_id", "message_id", "operation_id", "observed_ns"}, rows); err != nil {
		return err
	}
	measured, outcomes, useful, goodput := 0, map[string]int{}, int64(0), int64(0)
	measureEnd := int64(c.warmup + time.Duration(len(c.rates))*c.step)
	for _, x := range r.calls {
		if x.Step < 0 {
			continue
		}
		measured++
		outcomes[x.Outcome]++
		useful += x.Bytes
		if x.ResponseSeen && x.ResponseNS >= int64(c.warmup) && x.ResponseNS <= measureEnd {
			goodput += x.Bytes
		}
	}
	meta := map[string]any{"schema": 1, "run_id": r.id, "started": r.start.UTC(), "finished": end.UTC(), "measurement_start": r.start.Add(c.warmup).UTC(),
		"measurement_end": r.start.Add(c.warmup + time.Duration(len(c.rates))*c.step).UTC(), "seed": c.seed, "rates": c.rates, "media_bytes": c.mediaBytes, "kind_names": kindNames,
		"warmup_ns": int64(c.warmup), "step_ns": int64(c.step), "drain_ns": int64(c.drain), "timeout_ns": int64(c.callTimeout),
		"fixture": c.fixtureFile != "", "gateways": len(c.gateways), "users_per_gateway": c.users, "mix": c.mix, "max_inflight": c.maxInflight,
		"measurement_calls": measured, "measurement_outcomes": outcomes, "measurement_useful_bytes": useful, "goodput_bytes_per_second": float64(goodput) / (time.Duration(len(c.rates)) * c.step).Seconds(), "time_unit": "ns", "delivery_order_scope": "sender and recipient within chat", "planned": len(r.calls), "metrics_endpoints": c.metricURLs, "resource_collection": c.dockerProject != "", "complete": runErr == nil}
	if runErr != nil {
		meta["error"] = "run_failed"
		return atomicJSON(filepath.Join(dir, "failed.json"), meta)
	}
	return atomicJSON(filepath.Join(dir, "meta.json"), meta)
}

func releaseCall(x *callRecord) {
	if x.timer != nil {
		x.timer.Stop()
	}
	if x.release != nil {
		x.release()
		x.release = nil
	}
}

func (r *rawRecords) reserve(x *callRecord, slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
	default:
		r.finish(x, time.Now(), "not_sent", "inflight_limit", 0)
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	x.release = func() { <-slots }
	x.timer = time.AfterFunc(time.Until(r.start.Add(time.Duration(x.DeadlineNS))), func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if x.Outcome != "pending" && x.Outcome != "planned" {
			return
		}
		if !x.Sent {
			x.Outcome = "not_sent"
		} else {
			x.Outcome = "deadline"
		}
		x.Error, x.CompleteNS = "deadline_exceeded", x.DeadlineNS
		releaseCall(x)
	})
	return true
}
func (r *rawRecords) noteNotify(userID, id string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notifications = append(r.notifications, notifyObservation{userID, id, int64(now.Sub(r.start))})
	r.events = append(r.events, rawEvent{Kind: "notify_push", User: userID, MessageID: id, AtNS: int64(now.Sub(r.start))})
}

func (r *rawRecords) deliveryTallies() map[key]*deliveryTally {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := map[key]*deliveryTally{}
	for _, d := range r.receipts {
		if d.Step < 0 {
			continue
		}
		k := key{d.Step, d.Gateway, sendMessage}
		t := result[k]
		if t == nil {
			t = &deliveryTally{}
			result[k] = t
		}
		t.attempted++
		if d.Accepted {
			t.accepted++
			if d.Received && d.ID == d.AckID {
				t.received++
				t.latencies = append(t.latencies, time.Duration(d.ReceivedNS-d.ScheduledNS))
			} else {
				t.missing++
			}
		} else if d.Received {
			t.unconfirmed++
		}
		t.duplicates += d.Duplicates
		if d.Reordered {
			t.reordered++
		}
		if d.Mismatch {
			t.mismatches++
		}
	}
	return result
}

func optionalInteger(v int64, present bool) string {
	if !present {
		return ""
	}
	return integer(v)
}
