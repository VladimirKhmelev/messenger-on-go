package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/events"
)

var errPublishFailed = errors.New("publish failed")

func TestChatService_SendMessage_PublishesMessageCreated(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, []string{"user-a", "user-b"})

	publisher := newFakeEventPublisher()
	svc := NewChatService(repo, newFakeAuthClient(), publisher)

	message, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v", err)
	}

	if len(publisher.messageCreatedEvents) != 1 {
		t.Fatalf("SendMessage() published %d msg.created events, want 1", len(publisher.messageCreatedEvents))
	}
	event := publisher.messageCreatedEvents[0]
	if event.MessageID != message.ID || event.ChatID != message.ChatID || event.SenderID != message.SenderID {
		t.Errorf("SendMessage() published event = %+v, want to match message %+v", event, message)
	}
}

func TestChatService_SendMessage_EventPublishFailureDoesNotFailSend(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, []string{"user-a", "user-b"})

	svc := NewChatService(repo, newFakeAuthClient(), &failingEventPublisher{})

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if err != nil {
		t.Fatalf("SendMessage() unexpected error: %v, want nil (event publish failures must not fail sending)", err)
	}
}

type failingEventPublisher struct{}

func (failingEventPublisher) PublishMessageCreated(context.Context, events.MessageCreated) error {
	return errPublishFailed
}
