package events

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
	"github.com/nats-io/nats.go/jetstream"
)

// Exercise the actual mixed-subject consumer, not just the scheduler.
func TestConsumeMixedChatEventsInPublicationOrder(t *testing.T) {
	url := runNATS(t)
	_, js := connect(t, url)
	createStream(t, js)
	for name, subject := range map[string]string{notifyStreamName: subjectNotify, userStreamName: subjectProfileUpdated} {
		if _, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: name, Subjects: []string{subject}}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	got := make(chan string, 20)
	started := make(chan struct{})
	release := make(chan struct{})
	h := Handlers{
		OnMessageCreated: func(ctx context.Context, e domain.MessageCreated) error {
			if e.ChatID == "A" {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			got <- e.ChatID + ":created"
			return nil
		},
		OnMessageUpdated: func(_ context.Context, e domain.MessageUpdated) error {
			got <- fmt.Sprintf("%s:updated:%t", e.ChatID, e.Deleted)
			return nil
		},
		OnMessageRead: func(_ context.Context, e domain.MessageRead) error { got <- e.ChatID + ":read"; return nil },
		OnChatDeleted: func(_ context.Context, e domain.ChatDeleted) error { got <- e.ChatID + ":deleted"; return nil },
	}
	done := make(chan error, 1)
	go func() { done <- ConsumeWithConfig(ctx, url, h, Config{Workers: 2, QueueSize: 8, Timeout: time.Second}) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(wait):
			t.Error("consumer failed to stop")
		}
	})
	name := waitConsumer(t, js, "")
	stream, err := js.Stream(t.Context(), chatStreamName)
	if err != nil {
		t.Fatal(err)
	}
	c, err := stream.Consumer(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	if filters := c.CachedInfo().Config.FilterSubjects; len(filters) != 5 {
		t.Fatalf("filters = %v", filters)
	}
	send := func(subject, body string) {
		t.Helper()
		if _, err := js.Publish(t.Context(), subject, []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	send(subjectMsg, `{"chat_id":"A"}`)
	select {
	case <-started:
	case <-time.After(wait):
		t.Fatal("first event did not start")
	}
	send(subjectMsgEdit, `{"chat_id":"A"}`)
	send(subjectMsgDelete, `{"chat_id":"A","deleted":true}`)
	send(subjectMsgRead, `{"chat_id":"A"}`)
	send(subjectChatDelete, `{"chat_id":"A"}`)
	send(subjectMsg, `invalid JSON`)
	send(subjectMsg, `{}`)
	send(subjectMsg, `{"chat_id":"B"}`)
	receive(t, got, "B:created")
	close(release)
	for _, want := range []string{"A:created", "A:updated:false", "A:updated:true", "A:read", "A:deleted"} {
		receive(t, got, want)
	}
	if names := consumers(t, js); len(names) != 1 {
		t.Fatalf("CHAT_EVENTS consumers = %v", names)
	}
}
