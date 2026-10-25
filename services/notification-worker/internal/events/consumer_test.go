package events

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const wait = 10 * time.Second

// TestMain shortens the fetches, which a test waits out when it ends.
func TestMain(m *testing.M) {
	pullMaxWait = 200 * time.Millisecond
	os.Exit(m.Run())
}

// runNATS starts a NATS server with JetStream and returns its URL.
func runNATS(t *testing.T) string {
	t.Helper()
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	t.Cleanup(srv.Shutdown)
	if !srv.ReadyForConnections(wait) {
		t.Fatal("NATS is not ready")
	}
	return srv.ClientURL()
}

// connect returns JetStream on a connection of its own.
func connect(t *testing.T, url string) (*nats.Conn, jetstream.JetStream) {
	t.Helper()
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	return nc, js
}

func createStream(t *testing.T, js jetstream.JetStream) jetstream.Stream {
	t.Helper()
	stream, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "CHAT_EVENTS", Subjects: []string{"msg.*"}})
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func setRetryPause(t *testing.T, d time.Duration) {
	t.Helper()
	old := retryPause
	retryPause = d
	t.Cleanup(func() { retryPause = old })
}

// consume runs consumeOne until the test ends and returns the messages it
// hands over.
func consume(t *testing.T, js jetstream.JetStream) <-chan string {
	t.Helper()
	got := make(chan string, 10)
	sub := Subscription{StreamName: "CHAT_EVENTS", ConsumerName: "worker", FilterSubject: "msg.created",
		Handle: func(_ context.Context, _ string, data []byte) { got <- string(data) }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- consumeOne(ctx, js, sub) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("consumeOne = %v", err)
			}
		case <-time.After(wait):
			t.Error("consumeOne did not return")
		}
	})
	return got
}

// lines counts the lines logged that contain a text.
type lines struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	text string
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lines) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Count(l.buf.String(), l.text)
}

// logged captures the log until the test ends and counts the lines with a
// text.
func logged(t *testing.T, text string) *lines {
	t.Helper()
	l := &lines{text: text}
	log.SetOutput(l)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return l
}

// paced waits for a failure and checks that the worker then logs about one
// failure a pause over ten pauses.
func paced(t *testing.T, failures *lines, pause time.Duration) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for failures.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	before := failures.count()
	time.Sleep(10 * pause)
	if n := failures.count() - before; n < 3 || n > 15 {
		t.Errorf("%d failures logged in %v with a pause of %v", n, 10*pause, pause)
	}
}

// TestConsumeOnePausesOnFailures: the worker handles and acks the messages,
// and once its consumer is gone it retries once a pause, not in a busy loop.
func TestConsumeOnePausesOnFailures(t *testing.T) {
	pause := 100 * time.Millisecond
	setRetryPause(t, pause)
	failures := logged(t, "failed")
	_, js := connect(t, runNATS(t))
	stream := createStream(t, js)
	got := consume(t, js)

	if _, err := js.Publish(t.Context(), "msg.created", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-got:
		if data != "hello" {
			t.Fatalf("got %q", data)
		}
	case <-time.After(wait):
		t.Fatal("no message")
	}
	deadline := time.Now().Add(wait)
	for {
		c, err := stream.Consumer(t.Context(), "worker")
		if err != nil {
			t.Fatal(err)
		}
		if info, err := c.Info(t.Context()); err == nil && info.AckFloor.Consumer == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the message is not acked")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := stream.DeleteConsumer(t.Context(), "worker"); err != nil {
		t.Fatal(err)
	}
	paced(t, failures, pause)
}

// TestConsumeOnePausesWhenDisconnected: when even the fetch fails, as on a
// closed connection, the worker retries once a pause too.
func TestConsumeOnePausesWhenDisconnected(t *testing.T) {
	pause := 100 * time.Millisecond
	setRetryPause(t, pause)
	failures := logged(t, "failed")
	url := runNATS(t)
	_, js := connect(t, url)
	stream := createStream(t, js)
	nc, own := connect(t, url)
	consume(t, own)
	deadline := time.Now().Add(wait)
	for {
		if _, err := stream.Consumer(t.Context(), "worker"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no consumer")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The fetch under way waits out its expiry; the next ones fail.
	nc.Close()
	paced(t, failures, pause)
}

// TestConsumeOneStopsDuringPause: a stopping worker does not wait out the
// pause before a retry.
func TestConsumeOneStopsDuringPause(t *testing.T) {
	setRetryPause(t, time.Minute)
	nc, js := connect(t, runNATS(t))
	createStream(t, js)
	sub := Subscription{StreamName: "CHAT_EVENTS", ConsumerName: "worker", FilterSubject: "msg.created",
		Handle: func(context.Context, string, []byte) {}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- consumeOne(ctx, js, sub) }()
	time.Sleep(100 * time.Millisecond)
	nc.Close()
	// The fetch under way expires, the next one fails, and the pause begins.
	time.Sleep(pullMaxWait + 1500*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("consumeOne = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("consumeOne waits out the pause")
	}
}
