package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type kind int

const (
	getPresence kind = iota
	getReadStatus
	sendMessage
	kinds

	anyKind kind = -1
)

const (
	createChatKind kind = kinds
	mediaKind      kind = kinds + 1
)

var kindNames = [5]string{"get_presence", "get_read_status", "send_message", "create_chat", "media"}

// writeTimeout bounds a write to a gateway that no longer reads.
const writeTimeout = 10 * time.Second
const maxPlanned = 1000000

// mix holds the share of every kind of request; the shares sum to 1.
type mix [5]float64

func parseMix(s string) (mix, error) {
	var m mix
	var total float64
	for _, part := range strings.Split(s, ",") {
		name, weight, _ := strings.Cut(part, "=")
		k := slices.Index(kindNames[:], name)
		w, err := strconv.ParseFloat(weight, 64)
		if k < 0 || err != nil || w < 0 || math.IsNaN(w) || math.IsInf(w, 0) {
			return mix{}, fmt.Errorf("mix %q: want request=weight with a request of %s", part, strings.Join(kindNames[:], ", "))
		}
		m[k] += w
		total += w
	}
	if total == 0 || math.IsInf(total, 0) {
		return mix{}, fmt.Errorf("mix %q: all the weights are zero", s)
	}
	for k := range m {
		m[k] /= total
	}
	return m, nil
}

// pick chooses a kind by x, uniform in [0, 1).
func (m mix) pick(x float64) kind {
	last := anyKind
	for k, share := range m {
		if share == 0 {
			continue
		}
		last = kind(k)
		if x < share {
			return last
		}
		x -= share
	}
	return last
}

// arrival is a request due at an offset from the start of the load.
type arrival struct {
	at   time.Duration
	step int // the index of the step, -1 during the warm-up
	kind kind
}

// schedule lays out the requests of one gateway: Poisson arrivals at
// rates[i] a second during step i, after a warm-up at rates[0].
func schedule(rng *rand.Rand, rates []float64, warmup, step time.Duration, m mix) []arrival {
	var out []arrival
	phase := func(i int, rate float64, from, to time.Duration) {
		t := from
		for {
			gap := rng.ExpFloat64() / rate * float64(time.Second)
			if gap >= float64(to-t) || len(out) >= maxPlanned {
				return
			}
			t += max(time.Nanosecond, time.Duration(gap))
			if t >= to {
				return
			}
			out = append(out, arrival{at: t, step: i, kind: m.pick(rng.Float64())})
		}
	}
	phase(-1, rates[0], 0, warmup)
	for i, rate := range rates {
		from := warmup + time.Duration(i)*step
		phase(i, rate, from, from+step)
	}
	return out
}

// clientMessage is a request in the protocol of ws-gateway.
type clientMessage struct {
	Type       string `json:"type"`
	ChatID     string `json:"chat_id,omitempty"`
	Text       string `json:"text,omitempty"`
	PeerUserID string `json:"peer_user_id,omitempty"`
}

// serverMessage is what wsload reads of a frame from a gateway.
type serverMessage struct {
	Type       string `json:"type"`
	Error      string `json:"error"`
	PeerUserID string `json:"peer_user_id"`
	MessageID  string `json:"message_id"`
	ChatID     string `json:"chat_id"`
	Message    *struct {
		MessageID string `json:"message_id"`
		Text      string `json:"text"`
	} `json:"message"`
}

// answer tells whether a frame answers a request, and what kind of request;
// the other frames are notifications that a gateway pushes on its own, such
// as the messages the users send.
func answer(m serverMessage, idle string) (kind, bool) {
	switch m.Type {
	case "error":
		return anyKind, true
	case "message_sent":
		return sendMessage, true
	case "read_status":
		// Nobody marks anything read, so nobody is told of it.
		return getReadStatus, true
	case "presence_changed":
		// The idle user never connects, so its presence never changes.
		return getPresence, m.PeerUserID == idle
	}
	return 0, false
}

// conn is the connection of a user to a gateway. A gateway handles the
// requests of a connection one by one and answers them in order, so an
// answer belongs to the oldest request still without one.
type conn struct {
	ws   *websocket.Conn
	user user

	writeMu       sync.Mutex // writes one at a time, in the order of pending
	mu            sync.Mutex
	pending       []sent
	deliveries    map[string]*delivery
	nextMessage   uint64
	lastDelivered uint64
	queue         chan queued
	closed        bool
}

// sent is a request written to a connection.
type sent struct {
	arrival
	due      time.Time
	delivery *delivery
	raw      *callRecord
	token    string
}

// load is a run of wsload.
type load struct {
	c           config
	idle        string // a user that never connects, whose presence get_presence asks
	rec         *recorder
	sends       sync.WaitGroup
	raw         *rawRecords
	runID       string
	ending      atomic.Bool
	slots       chan struct{}
	mediaClient *http.Client
}

type queued struct {
	arrival arrival
	row     *callRecord
}

func run(ctx context.Context, c config) (rep *report, err error) {
	if c.callTimeout == 0 {
		c.callTimeout = writeTimeout
	}
	if c.maxInflight == 0 {
		c.maxInflight = 10000
	}
	if c.mediaBytes == 0 {
		c.mediaBytes = 65536
	}
	if c.out != "" {
		if err = os.MkdirAll(filepath.Dir(c.out), 0700); err != nil {
			return nil, err
		}
		if err = os.Mkdir(c.out, 0700); err != nil {
			return nil, fmt.Errorf("output directory must be new: %w", err)
		}
	}
	l := &load{c: c, idle: newID(), rec: newRecorder(), raw: newRaw(time.Now()), runID: newID(), slots: make(chan struct{}, c.maxInflight), mediaClient: mediaClient(c.minioAddr)}
	l.runID = l.raw.id
	defer l.mediaClient.CloseIdleConnections()
	conns := make([][]*conn, len(c.gateways))
	var readers sync.WaitGroup
	closeAll := func() {
		for _, cs := range conns {
			for _, cn := range cs {
				_ = cn.ws.Close()
			}
		}
		readers.Wait()
	}
	defer func() {
		l.ending.Store(true)
		closeAll()
		now := time.Now()
		l.raw.close(now, err != nil, nil)
		rep = l.rec.report(c, l.raw.start)
		rep.raw = l.raw
		if c.fixtureFile != "" {
			rep.deliveries = l.raw.deliveryTallies()
		}
		if c.out != "" {
			err = errors.Join(err, l.raw.save(c.out, c, now, err))
		}
	}()
	var users [][]user
	if c.fixtureFile != "" {
		users, err = readFixture(c.fixtureFile, len(c.gateways))
		if err != nil {
			return nil, err
		}
		c.users = len(users[0])
		l.c = c
	} else {
		users = make([][]user, len(c.gateways))
		expires := time.Now().Add(c.warmup + time.Duration(len(c.rates))*c.step + c.drain + 10*time.Minute)
		for g := range c.gateways {
			for range c.users {
				u, e := newUser(ctx, c.chatURL, c.secret, expires)
				if e != nil {
					return nil, e
				}
				users[g] = append(users[g], u)
			}
		}
	}
	for g, gw := range c.gateways {
		for _, u := range users[g] {
			ws, e := dial(ctx, gw.url, u.token)
			if e != nil {
				return nil, fmt.Errorf("%s: connect failed", gw.name)
			}
			cn := &conn{ws: ws, user: u, queue: make(chan queued, min(c.maxInflight, 64))}
			conns[g] = append(conns[g], cn)
			readers.Go(func() { l.read(cn, g) })
		}
	}
	if c.fixtureFile != "" {
		ready, stop := context.WithTimeout(ctx, 30*time.Second)
		err = fixtureReady(ready, c)
		stop()
		if err != nil {
			return nil, err
		}
	}
	arrivals := make([][]arrival, len(c.gateways))
	rows := make([][]*callRecord, len(c.gateways))
	perGateway := make([]float64, len(c.rates))
	for i, rate := range c.rates {
		perGateway[i] = rate / float64(len(c.gateways))
	}
	for g := range c.gateways {
		arrivals[g] = schedule(rand.New(rand.NewPCG(c.seed, uint64(g))), perGateway, c.warmup, c.step, c.mix)
		if len(arrivals[g]) >= maxPlanned || len(l.raw.calls)+len(arrivals[g]) > maxPlanned {
			return nil, fmt.Errorf("schedule exceeds %d operations", maxPlanned)
		}
		var next, nextSender int
		for n, a := range arrivals[g] {
			index := next % len(conns[g])
			next++
			if a.kind == sendMessage {
				index = nextSender % len(conns[g])
				nextSender++
			}
			cn := conns[g][index]
			x := l.raw.plan(g, n, a, cn.user, c.callTimeout)
			rows[g] = append(rows[g], x)
			if a.kind == sendMessage {
				receiver := cn.user.peerID
				if receiver == "" {
					receiver = cn.user.id
				}
				l.raw.expect(x, l.runID+":"+x.ID, receiver, uint64(n+1))
			}
		}
	}
	start := time.Now()
	l.raw.mu.Lock()
	l.raw.start = start
	l.raw.mu.Unlock()
	resources := make(chan error, 1)
	go func() { resources <- captureResources(ctx, c, start) }()
	var workers sync.WaitGroup
	for g, cs := range conns {
		for _, cn := range cs {
			workers.Go(func() {
				for q := range cn.queue {
					l.send(cn, g, q.arrival, start.Add(q.arrival.at), q.row)
					l.sends.Done()
				}
			})
		}
	}
	var drivers sync.WaitGroup
	for g := range conns {
		drivers.Go(func() { l.drive(ctx, g, conns[g], arrivals[g], rows[g], start) })
	}
	drivers.Wait()
	for _, cs := range conns {
		for _, cn := range cs {
			close(cn.queue)
		}
	}
	if ctx.Err() != nil {
		l.ending.Store(true)
		closeAll()
	}
	workers.Wait()
	l.sends.Wait()
	if ctx.Err() == nil {
		timer := time.NewTimer(c.drain)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	err = errors.Join(ctx.Err(), <-resources)
	l.ending.Store(true)
	closeAll()
	for g, cs := range conns {
		for _, cn := range cs {
			for _, q := range cn.pending {
				l.rec.lost(q.step, g, q.kind)
			}
			for _, d := range cn.deliveries {
				l.rec.delivery(g, d)
			}
		}
	}
	return rep, err
}

// drive sends the requests of a gateway when they are due, each without
// waiting for the others. The users take turns, the senders of messages
// apart, so that each sends an equal share of them.
func (l *load) drive(ctx context.Context, g int, conns []*conn, arrivals []arrival, rows []*callRecord, start time.Time) {
	var next, nextSender int
	for n, a := range arrivals {
		due := start.Add(a.at)
		if d := time.Until(due); d > 0 {
			timer := time.NewTimer(d)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return
		}
		index := next % len(conns)
		next++
		if a.kind == sendMessage {
			index = nextSender % len(conns)
			nextSender++
		}
		cn := conns[index]
		x := rows[n]
		if !l.raw.reserve(x, l.slots) {
			continue
		}
		l.sends.Add(1)
		if a.kind >= kinds {
			go func() { defer l.sends.Done(); l.http(ctx, cn, g, a, due, x) }()
		} else {
			select {
			case cn.queue <- queued{a, x}:
			default:
				l.raw.finish(x, time.Now(), "not_sent", "write_queue_full", 0)
				l.sends.Done()
			}
		}
	}
}

func (l *load) http(ctx context.Context, c *conn, g int, a arrival, due time.Time, x *callRecord) {
	call, cancel := context.WithDeadline(ctx, due.Add(l.c.callTimeout))
	defer cancel()
	if !l.raw.send(x, time.Now()) {
		return
	}
	l.rec.sent(a.step, g, a.kind)
	var err error
	var bytes int64
	if a.kind == createChatKind {
		err = lookupChat(call, l.c, c.user)
	} else {
		bytes, err = transferMedia(call, l.mediaClient, l.c, c.user, x.ID)
	}
	outcome, failure := "success", ""
	if err != nil {
		outcome, failure = "rpc_error", err.Error()
	}
	if call.Err() != nil {
		outcome, failure = "deadline", "deadline_exceeded"
	}
	now := time.Now()
	l.raw.finish(x, now, outcome, failure, bytes)
	l.rec.answer(a.step, g, a.kind, now.Sub(due), failure)
}

func (l *load) send(c *conn, g int, a arrival, due time.Time, x *callRecord) {
	m := clientMessage{Type: kindNames[a.kind]}
	switch a.kind {
	case getPresence:
		m.PeerUserID = l.idle
	case getReadStatus:
		m.ChatID, m.PeerUserID = c.user.chatID, c.user.id
		if c.user.peerID != "" {
			m.PeerUserID = c.user.peerID
		}
	case sendMessage:
		m.ChatID, m.Text = c.user.chatID, "wsload"
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		l.raw.finish(x, time.Now(), "not_sent", "connection_closed", 0)
		return
	}
	if !l.raw.send(x, time.Now()) {
		c.mu.Unlock()
		return
	}
	sent := sent{arrival: a, due: due, raw: x, token: l.runID + ":" + x.ID}
	if a.kind == sendMessage {
		c.nextMessage++
		m.Text = sent.token
		if c.deliveries == nil {
			c.deliveries = make(map[string]*delivery)
		}
		d := &delivery{step: a.step, due: due, order: c.nextMessage}
		c.deliveries[m.Text] = d
		sent.delivery = d
	}
	c.pending = append(c.pending, sent)
	c.mu.Unlock()
	l.rec.sent(a.step, g, a.kind)
	data, _ := json.Marshal(m) // strings only: it cannot fail
	// A request that fails to go out gets no answer and counts as lost.
	_ = c.ws.SetWriteDeadline(due.Add(l.c.callTimeout))
	if err := c.ws.WriteMessage(websocket.TextMessage, data); err != nil {
		l.raw.finish(x, time.Now(), "write_error", "websocket_write_failed", 0)
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		_ = c.ws.Close()
	}
}

// read matches the answers on a connection to its requests until the
// connection closes.
func (l *load) read(c *conn, g int) {
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			c.mu.Lock()
			c.closed = true
			if !l.ending.Load() {
				for _, s := range c.pending {
					l.raw.finish(s.raw, time.Now(), "connection_closed", "websocket_closed", 0)
				}
			}
			c.mu.Unlock()
			return
		}
		var m serverMessage
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.Type == "message_received" {
			c.received(m, time.Now())
			l.raw.receive(c.user.id, m, time.Now())
			continue
		}
		if m.Type == "notify_push" {
			l.raw.noteNotify(c.user.id, m.MessageID, time.Now())
			continue
		}
		k, ok := answer(m, l.idle)
		if !ok {
			continue
		}
		now := time.Now()
		c.mu.Lock()
		if len(c.pending) == 0 {
			c.mu.Unlock()
			l.rec.stray()
			continue
		}
		s := c.pending[0]
		c.pending = c.pending[1:]

		failure := m.Error
		switch {
		case k != anyKind && k != s.kind:
			failure = fmt.Sprintf("%s answered with %s", kindNames[s.kind], m.Type)
		case k == anyKind && failure == "":
			failure = "an error without a text"
		case k == sendMessage && failure == "" && m.MessageID == "":
			failure = "message_sent without a message_id"
		}
		if s.delivery != nil {
			s.delivery.accepted = failure == "" && k == sendMessage && m.MessageID != ""
			s.delivery.ackID = m.MessageID
			if s.delivery.received && s.delivery.accepted && s.delivery.id != m.MessageID {
				s.delivery.mismatch = true
			}
		}
		c.mu.Unlock()
		l.raw.ack(s.raw, s.token, m.MessageID, failure == "" && k == sendMessage)
		outcome := "success"
		if failure != "" {
			outcome = "rpc_error"
		}
		l.raw.finish(s.raw, now, outcome, failure, 0)
		l.rec.answer(s.step, g, s.kind, now.Sub(s.due), failure)
	}
}
