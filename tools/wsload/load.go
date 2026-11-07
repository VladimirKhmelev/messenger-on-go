package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
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

var kindNames = [kinds]string{"get_presence", "get_read_status", "send_message"}

// writeTimeout bounds a write to a gateway that no longer reads.
const writeTimeout = 10 * time.Second

// mix holds the share of every kind of request; the shares sum to 1.
type mix [kinds]float64

func parseMix(s string) (mix, error) {
	var m mix
	var total float64
	for _, part := range strings.Split(s, ",") {
		name, weight, _ := strings.Cut(part, "=")
		k := slices.Index(kindNames[:], name)
		w, err := strconv.ParseFloat(weight, 64)
		if k < 0 || err != nil || w < 0 {
			return mix{}, fmt.Errorf("mix %q: want request=weight with a request of %s", part, strings.Join(kindNames[:], ", "))
		}
		m[k] += w
		total += w
	}
	if total == 0 {
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
			t += time.Duration(rng.ExpFloat64() / rate * float64(time.Second))
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
}

// sent is a request written to a connection.
type sent struct {
	arrival
	due      time.Time
	delivery *delivery
}

// load is a run of wsload.
type load struct {
	c     config
	idle  string // a user that never connects, whose presence get_presence asks
	rec   *recorder
	sends sync.WaitGroup
}

func run(ctx context.Context, c config) (*report, error) {
	l := &load{c: c, idle: newID(), rec: newRecorder()}
	length := c.warmup + time.Duration(len(c.rates))*c.step
	// The tokens outlive the setup and the load.
	expires := time.Now().Add(length + c.drain + 10*time.Minute)

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
	defer closeAll()
	for g, gw := range c.gateways {
		for range c.users {
			u, err := newUser(ctx, c.chatURL, c.secret, expires)
			if err != nil {
				return nil, err
			}
			ws, err := dial(ctx, gw.url, u.token)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", gw.name, err)
			}
			cn := &conn{ws: ws, user: u}
			conns[g] = append(conns[g], cn)
			readers.Go(func() { l.read(cn, g) })
		}
	}
	log.Printf("%d users on each of %d gateways; warm-up for %v", c.users, len(c.gateways), c.warmup)

	start := time.Now()
	for i, rate := range c.rates {
		t := time.AfterFunc(c.warmup+time.Duration(i)*c.step, func() {
			log.Printf("step %d of %d: %g rps", i+1, len(c.rates), rate)
		})
		defer t.Stop()
	}
	perGateway := make([]float64, len(c.rates))
	for i, rate := range c.rates {
		perGateway[i] = rate / float64(len(c.gateways))
	}
	var drivers sync.WaitGroup
	for g := range c.gateways {
		arrivals := schedule(rand.New(rand.NewPCG(c.seed, uint64(g))), perGateway, c.warmup, c.step, c.mix) //nolint:gosec // a reproducible load, not a secret
		drivers.Go(func() { l.drive(ctx, g, conns[g], arrivals, start) })
	}
	drivers.Wait()
	l.sends.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Observe the full drain window: a delivery or duplicate may arrive
	// after the last request acknowledgement.
	timer := time.NewTimer(c.drain)
	select {
	case <-ctx.Done():
		timer.Stop()
		return nil, ctx.Err()
	case <-timer.C:
	}

	closeAll()
	for g, cs := range conns {
		for _, cn := range cs {
			for _, s := range cn.pending {
				l.rec.lost(s.step, g, s.kind)
			}
			for _, d := range cn.deliveries {
				l.rec.delivery(g, d)
			}
		}
	}
	return l.rec.report(c, start), nil
}

// drive sends the requests of a gateway when they are due, each without
// waiting for the others. The users take turns, the senders of messages
// apart, so that each sends an equal share of them.
func (l *load) drive(ctx context.Context, g int, conns []*conn, arrivals []arrival, start time.Time) {
	var next, nextSender int
	for _, a := range arrivals {
		due := start.Add(a.at)
		if d := time.Until(due); d > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d):
			}
		}
		var c *conn
		if a.kind == sendMessage {
			c = conns[nextSender%len(conns)]
			nextSender++
		} else {
			c = conns[next%len(conns)]
			next++
		}
		l.sends.Go(func() { l.send(c, g, a, due) })
	}
}

func (l *load) send(c *conn, g int, a arrival, due time.Time) {
	m := clientMessage{Type: kindNames[a.kind]}
	switch a.kind {
	case getPresence:
		m.PeerUserID = l.idle
	case getReadStatus:
		m.ChatID, m.PeerUserID = c.user.chatID, c.user.id
	case sendMessage:
		m.ChatID, m.Text = c.user.chatID, "wsload"
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	sent := sent{arrival: a, due: due}
	if a.kind == sendMessage {
		c.nextMessage++
		m.Text = "wsload:" + strconv.FormatUint(c.nextMessage, 10)
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
	_ = c.ws.SetWriteDeadline(time.Now().Add(writeTimeout))
	_ = c.ws.WriteMessage(websocket.TextMessage, data)
}

// read matches the answers on a connection to its requests until the
// connection closes.
func (l *load) read(c *conn, g int) {
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		var m serverMessage
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.Type == "message_received" {
			c.received(m, time.Now())
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
		l.rec.answer(s.step, g, s.kind, now.Sub(s.due), failure)
	}
}
