package notify

import (
	"context"

	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/domain"
)

type ChatClient interface {
	ListMembers(ctx context.Context, chatID string) ([]string, error)
	IsOnline(ctx context.Context, userID string) (bool, error)
}

type AuthClient interface {
	ListPushSubscriptions(ctx context.Context, userID string) ([]domain.PushSubscription, error)
}

type WebPushSender interface {
	Send(ctx context.Context, sub domain.PushSubscription, payload domain.PushPayload)
}

type EventPublisher interface {
	PublishNotifyPush(ctx context.Context, event domain.NotifyPush) error
}
