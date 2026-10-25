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

// runJetStream starts a NATS server with the stream of chat events as
// chat-service creates it.
func runJetStream(t *testing.T) jetstream.JetStream {
	t.Helper()
	_, js := connect(t, runNATS(t))
	createStream(t, js)
	return js
}

func createStream(t *testing.T, js jetstream.JetStream) {
	t.Helper()
	if _, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: chatStreamName, Subjects: []string{"msg.*", "chat.*"}}); err != nil {
		t.Fatal(err)
	}
}

// consume runs consumeOne on msg.created until the test ends and returns
// the messages it hands over.
func consume(t *testing.T, js jetstream.JetStream) <-chan string {
	t.Helper()
	got := make(chan string, 100)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- consumeOne(ctx, js, chatStreamName, subjectMsg, func(_ context.Context, data []byte) { got <- string(data) })
	}()
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

// consumers returns the names of the consumers of the stream.
func consumers(t *testing.T, js jetstream.JetStream) []string {
	t.Helper()
	stream, err := js.Stream(t.Context(), chatStreamName)
	if err != nil {
		return nil
	}
	var names []string
	lister := stream.ConsumerNames(t.Context())
	for name := range lister.Name() {
		names = append(names, name)
	}
	return names
}

// waitConsumer waits for a consumer of the stream other than old.
func waitConsumer(t *testing.T, js jetstream.JetStream, old string) string {
	t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		for _, name := range consumers(t, js) {
			if name != old {
				return name
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no consumer")
	return ""
}

func publish(t *testing.T, js jetstream.JetStream, data string) {
	t.Helper()
	if _, err := js.Publish(t.Context(), subjectMsg, []byte(data)); err != nil {
		t.Fatal(err)
	}
}

func receive(t *testing.T, got <-chan string, want string) {
	t.Helper()
	select {
	case data := <-got:
		if data != want {
			t.Fatalf("got %q, want %q", data, want)
		}
	case <-time.After(wait):
		t.Fatalf("no %q", want)
	}
}

func setRetryPause(t *testing.T, d time.Duration) {
	t.Helper()
	old := retryPause
	retryPause = d
	t.Cleanup(func() { retryPause = old })
}

// TestMain shortens the fetches, which a test waits out when it ends.
func TestMain(m *testing.M) {
	pullMaxWait = 200 * time.Millisecond
	os.Exit(m.Run())
}

// TestConsumeOnlyNew: a starting gateway does not fan out the events
// already in the stream, and its consumer outlives slow batches.
func TestConsumeOnlyNew(t *testing.T) {
	js := runJetStream(t)
	publish(t, js, "old 1")
	publish(t, js, "old 2")
	got := consume(t, js)
	name := waitConsumer(t, js, "")
	publish(t, js, "new")
	receive(t, got, "new")

	stream, err := js.Stream(t.Context(), chatStreamName)
	if err != nil {
		t.Fatal(err)
	}
	c, err := stream.Consumer(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	if cfg := c.CachedInfo().Config; cfg.DeliverPolicy != jetstream.DeliverNewPolicy || cfg.InactiveThreshold != inactiveThreshold {
		t.Errorf("consumer delivers %v and lives %v idle", cfg.DeliverPolicy, cfg.InactiveThreshold)
	}
	select {
	case data := <-got:
		t.Errorf("got %q too", data)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestConsumeRecreatesDeletedConsumer: when NATS deletes the consumer, as
// it does after inactiveThreshold, the gateway creates another and goes on.
func TestConsumeRecreatesDeletedConsumer(t *testing.T) {
	setRetryPause(t, 10*time.Millisecond)
	js := runJetStream(t)
	got := consume(t, js)
	name := waitConsumer(t, js, "")
	stream, err := js.Stream(t.Context(), chatStreamName)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.DeleteConsumer(t.Context(), name); err != nil {
		t.Fatal(err)
	}
	waitConsumer(t, js, name)
	publish(t, js, "after")
	receive(t, got, "after")
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

// paced waits for a failure and checks that the consumer then logs about
// one failure a pause over ten pauses.
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

// TestConsumePausesOnFailures: while the stream is gone the gateway retries
// once a pause, not in a busy loop, and resumes once the stream is back.
func TestConsumePausesOnFailures(t *testing.T) {
	pause := 100 * time.Millisecond
	setRetryPause(t, pause)
	failures := logged(t, "failed")
	js := runJetStream(t)
	got := consume(t, js)
	waitConsumer(t, js, "")
	if err := js.DeleteStream(t.Context(), chatStreamName); err != nil {
		t.Fatal(err)
	}
	paced(t, failures, pause)
	createStream(t, js)
	waitConsumer(t, js, "")
	publish(t, js, "back")
	receive(t, got, "back")
}

func TestConsumeWithoutStream(t *testing.T) {
	js := runJetStream(t)
	if err := consumeOne(t.Context(), js, "NO_SUCH_STREAM", subjectMsg, func(context.Context, []byte) {}); err == nil {
		t.Error("consumeOne started without its stream")
	}
}

// TestConsumePausesWhenDisconnected: when even the fetch fails, as on a
// closed connection, the gateway retries once a pause too.
func TestConsumePausesWhenDisconnected(t *testing.T) {
	pause := 100 * time.Millisecond
	setRetryPause(t, pause)
	failures := logged(t, "failed")
	url := runNATS(t)
	_, js := connect(t, url)
	createStream(t, js)
	nc, own := connect(t, url)
	consume(t, own)
	waitConsumer(t, js, "")
	// The fetch under way waits out its expiry; the next ones fail.
	nc.Close()
	paced(t, failures, pause)
}

// TestConsumeStopsDuringPause: a stopping gateway does not wait out the
// pause before a retry.
func TestConsumeStopsDuringPause(t *testing.T) {
	setRetryPause(t, time.Minute)
	js := runJetStream(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- consumeOne(ctx, js, chatStreamName, subjectMsg, func(context.Context, []byte) {}) }()
	waitConsumer(t, js, "")
	if err := js.DeleteStream(t.Context(), chatStreamName); err != nil {
		t.Fatal(err)
	}
	// The fetch under way fails, and the pause begins.
	time.Sleep(pullMaxWait + 300*time.Millisecond)
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
