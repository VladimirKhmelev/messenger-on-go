package notify

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/authclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/webpush"
)

type fakeChatClient struct {
	members map[string][]string
	online  map[string]bool
	listErr error
}

func newFakeChatClient() *fakeChatClient {
	return &fakeChatClient{members: map[string][]string{}, online: map[string]bool{}}
}

func (c *fakeChatClient) ListMembers(_ context.Context, chatID string) ([]string, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.members[chatID], nil
}

func (c *fakeChatClient) IsOnline(_ context.Context, userID string) (bool, error) {
	return c.online[userID], nil
}

type fakeAuthClient struct {
	subs map[string][]authclient.PushSubscription
}

func newFakeAuthClient() *fakeAuthClient {
	return &fakeAuthClient{subs: map[string][]authclient.PushSubscription{}}
}

func (c *fakeAuthClient) ListPushSubscriptions(_ context.Context, userID string) ([]authclient.PushSubscription, error) {
	return c.subs[userID], nil
}

type sentPush struct {
	sub     webpush.Subscription
	payload webpush.Payload
}

type fakeWebPushSender struct {
	sent []sentPush
}

func newFakeWebPushSender() *fakeWebPushSender {
	return &fakeWebPushSender{}
}

func (s *fakeWebPushSender) Send(_ context.Context, sub webpush.Subscription, payload webpush.Payload) {
	s.sent = append(s.sent, sentPush{sub: sub, payload: payload})
}

type fakeEventPublisher struct {
	published []events.NotifyPush
	err       error
}

func newFakeEventPublisher() *fakeEventPublisher {
	return &fakeEventPublisher{}
}

func (p *fakeEventPublisher) PublishNotifyPush(_ context.Context, event events.NotifyPush) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, event)
	return nil
}

func TestHandler_HandleMessageCreated_NotifiesOfflineNonSenderMembers(t *testing.T) {
	chat := newFakeChatClient()
	chat.members["chat-1"] = []string{"sender", "offline-user", "online-user"}
	chat.online["online-user"] = true

	auth := newFakeAuthClient()
	push := newFakeWebPushSender()
	publisher := newFakeEventPublisher()

	h := NewHandler(chat, auth, push, publisher)

	event := events.MessageCreated{
		MessageID: "msg-1",
		ChatID:    "chat-1",
		SenderID:  "sender",
		CreatedAt: time.Now(),
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	h.HandleMessageCreated(context.Background(), "msg.created", data)

	if len(publisher.published) != 1 {
		t.Fatalf("PublishNotifyPush called %d times, want 1", len(publisher.published))
	}
	if publisher.published[0].UserID != "offline-user" {
		t.Errorf("notified user = %q, want %q", publisher.published[0].UserID, "offline-user")
	}
}

func TestHandler_HandleMessageCreated_SendsWebPushToOfflineUserSubscriptions(t *testing.T) {
	chat := newFakeChatClient()
	chat.members["chat-1"] = []string{"sender", "offline-user"}

	auth := newFakeAuthClient()
	auth.subs["offline-user"] = []authclient.PushSubscription{
		{Endpoint: "https://push.example.com/a", P256dhKey: "p256dh-a", AuthKey: "auth-a"},
		{Endpoint: "https://push.example.com/b", P256dhKey: "p256dh-b", AuthKey: "auth-b"},
	}

	push := newFakeWebPushSender()
	publisher := newFakeEventPublisher()

	h := NewHandler(chat, auth, push, publisher)

	event := events.MessageCreated{MessageID: "msg-1", ChatID: "chat-1", SenderID: "sender", CreatedAt: time.Now()}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	h.HandleMessageCreated(context.Background(), "msg.created", data)

	if len(push.sent) != 2 {
		t.Fatalf("webpush.Send called %d times, want 2", len(push.sent))
	}
	for _, s := range push.sent {
		if s.payload.ChatID != "chat-1" || s.payload.MessageID != "msg-1" {
			t.Errorf("push payload = %+v, want ChatID=chat-1 MessageID=msg-1", s.payload)
		}
	}
	if push.sent[0].sub.Endpoint != "https://push.example.com/a" || push.sent[1].sub.Endpoint != "https://push.example.com/b" {
		t.Errorf("push sent to unexpected endpoints: %+v", push.sent)
	}
}

func TestHandler_HandleMessageCreated_NoSubscriptionsSendsNoPush(t *testing.T) {
	chat := newFakeChatClient()
	chat.members["chat-1"] = []string{"sender", "offline-user"}

	auth := newFakeAuthClient()
	push := newFakeWebPushSender()
	publisher := newFakeEventPublisher()

	h := NewHandler(chat, auth, push, publisher)

	event := events.MessageCreated{MessageID: "msg-1", ChatID: "chat-1", SenderID: "sender", CreatedAt: time.Now()}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	h.HandleMessageCreated(context.Background(), "msg.created", data)

	if len(push.sent) != 0 {
		t.Errorf("webpush.Send called %d times, want 0 (no subscriptions)", len(push.sent))
	}
}

func TestHandler_HandleMessageCreated_OnlineUsersSkipped(t *testing.T) {
	chat := newFakeChatClient()
	chat.members["chat-1"] = []string{"sender", "online-user"}
	chat.online["online-user"] = true

	auth := newFakeAuthClient()
	auth.subs["online-user"] = []authclient.PushSubscription{
		{Endpoint: "https://push.example.com/a", P256dhKey: "p256dh", AuthKey: "auth"},
	}

	push := newFakeWebPushSender()
	publisher := newFakeEventPublisher()

	h := NewHandler(chat, auth, push, publisher)

	event := events.MessageCreated{MessageID: "msg-1", ChatID: "chat-1", SenderID: "sender", CreatedAt: time.Now()}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	h.HandleMessageCreated(context.Background(), "msg.created", data)

	if len(publisher.published) != 0 {
		t.Errorf("PublishNotifyPush called %d times, want 0 (recipient is online)", len(publisher.published))
	}
	if len(push.sent) != 0 {
		t.Errorf("webpush.Send called %d times, want 0 (recipient is online)", len(push.sent))
	}
}

func TestHandler_HandleMessageCreated_InvalidJSON(t *testing.T) {
	chat := newFakeChatClient()
	auth := newFakeAuthClient()
	push := newFakeWebPushSender()
	publisher := newFakeEventPublisher()

	h := NewHandler(chat, auth, push, publisher)

	h.HandleMessageCreated(context.Background(), "msg.created", []byte("not json"))

	if len(publisher.published) != 0 || len(push.sent) != 0 {
		t.Error("HandleMessageCreated() with invalid JSON should not notify or push anyone")
	}
}

func TestHandler_HandleMessageCreated_ListMembersError(t *testing.T) {
	chat := newFakeChatClient()
	chat.listErr = errors.New("chat-service unavailable")

	auth := newFakeAuthClient()
	push := newFakeWebPushSender()
	publisher := newFakeEventPublisher()

	h := NewHandler(chat, auth, push, publisher)

	event := events.MessageCreated{MessageID: "msg-1", ChatID: "chat-1", SenderID: "sender", CreatedAt: time.Now()}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error: %v", err)
	}

	h.HandleMessageCreated(context.Background(), "msg.created", data)

	if len(publisher.published) != 0 || len(push.sent) != 0 {
		t.Error("HandleMessageCreated() should not notify or push anyone when ListMembers fails")
	}
}
