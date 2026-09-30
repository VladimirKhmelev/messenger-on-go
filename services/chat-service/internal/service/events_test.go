package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/domain"
)

var errPublishFailed = errors.New("publish failed")

func TestChatService_SendMessage_PublishesMessageCreated(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	publisher := newFakeEventPublisher()
	svc := NewChatService(repo, newFakeAuthClient(), publisher, newFakePresenceChecker(), newFakeRateLimiter())

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

func TestChatService_SendMessage_EventFailureFailsSend(t *testing.T) {
	repo := newFakeChatRepository()
	chat := &domain.Chat{ID: uuid.NewString(), CreatedAt: time.Now()}
	_ = repo.CreateChat(context.Background(), chat, chatKeys("user-a", "user-b"))

	svc := NewChatService(repo, newFakeAuthClient(), &failingEventPublisher{}, newFakePresenceChecker(), newFakeRateLimiter())

	_, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hello")
	if !errors.Is(err, errPublishFailed) {
		t.Fatalf("SendMessage() error = %v, want %v", err, errPublishFailed)
	}
}

type failingEventPublisher struct{}

func (failingEventPublisher) PublishMessageCreated(context.Context, domain.MessageCreated) error {
	return errPublishFailed
}

func (failingEventPublisher) PublishMessageUpdated(context.Context, domain.MessageUpdated) error {
	return errPublishFailed
}

func (failingEventPublisher) PublishMessageRead(context.Context, domain.MessageRead) error {
	return errPublishFailed
}

func (failingEventPublisher) PublishChatDeleted(context.Context, domain.ChatDeleted) error {
	return errPublishFailed
}

var errOutsideTx = errors.New("event published outside WithTx")

type txOnlyPublisher struct{ fakeEventPublisher }

func requireTx(ctx context.Context) error {
	if ctx.Value(fakeTxKey{}) == nil {
		return errOutsideTx
	}
	return nil
}

func (p *txOnlyPublisher) PublishMessageCreated(ctx context.Context, e domain.MessageCreated) error {
	if err := requireTx(ctx); err != nil {
		return err
	}
	return p.fakeEventPublisher.PublishMessageCreated(ctx, e)
}

func (p *txOnlyPublisher) PublishMessageUpdated(ctx context.Context, e domain.MessageUpdated) error {
	if err := requireTx(ctx); err != nil {
		return err
	}
	return p.fakeEventPublisher.PublishMessageUpdated(ctx, e)
}

func (p *txOnlyPublisher) PublishMessageRead(ctx context.Context, e domain.MessageRead) error {
	if err := requireTx(ctx); err != nil {
		return err
	}
	return p.fakeEventPublisher.PublishMessageRead(ctx, e)
}

func (p *txOnlyPublisher) PublishChatDeleted(ctx context.Context, e domain.ChatDeleted) error {
	if err := requireTx(ctx); err != nil {
		return err
	}
	return p.fakeEventPublisher.PublishChatDeleted(ctx, e)
}

func TestChatService_EventsArePublishedInsideTransaction(t *testing.T) {
	tests := []struct {
		name string
		run  func(svc *ChatService, repo *fakeChatRepository) error
	}{
		{"SendMessage", func(svc *ChatService, repo *fakeChatRepository) error {
			chat := newFakeGroupChat(repo, "user-a", "user-b")
			_, err := svc.SendMessage(context.Background(), chat.ID, "user-a", "hi")
			return err
		}},
		{"EditMessage", func(svc *ChatService, repo *fakeChatRepository) error {
			chat := newFakeGroupChat(repo, "user-a", "user-b")
			msg := seedMessage(repo, chat.ID, "user-a")
			_, err := svc.EditMessage(context.Background(), msg.ID, "user-a", "edited")
			return err
		}},
		{"DeleteMessageForAll", func(svc *ChatService, repo *fakeChatRepository) error {
			chat := newFakeGroupChat(repo, "user-a", "user-b")
			msg := seedMessage(repo, chat.ID, "user-a")
			return svc.DeleteMessageForAll(context.Background(), msg.ID, "user-a")
		}},
		{"MarkRead", func(svc *ChatService, repo *fakeChatRepository) error {
			chat := newFakeGroupChat(repo, "user-a", "user-b")
			msg := seedMessage(repo, chat.ID, "user-a")
			return svc.MarkRead(context.Background(), chat.ID, "user-b", msg.ID)
		}},
		{"DeleteGroupChat", func(svc *ChatService, repo *fakeChatRepository) error {
			chat := newFakeGroupChat(repo, "user-a", "user-b")
			return svc.DeleteGroupChat(context.Background(), chat.ID, "user-a")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeChatRepository()
			svc := NewChatService(repo, newFakeAuthClient(), &txOnlyPublisher{}, newFakePresenceChecker(), newFakeRateLimiter())
			if err := tt.run(svc, repo); err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
		})
	}
}

func seedMessage(repo *fakeChatRepository, chatID, senderID string) *domain.Message {
	msg := &domain.Message{ID: uuid.NewString(), ChatID: chatID, SenderID: senderID, Body: "hello", CreatedAt: time.Now()}
	_ = repo.CreateMessage(context.Background(), msg)
	return msg
}
