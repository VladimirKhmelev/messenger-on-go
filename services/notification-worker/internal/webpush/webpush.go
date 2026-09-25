package webpush

import (
	"context"
	"encoding/json"
	"io"
	"log"

	upstream "github.com/SherClockHolmes/webpush-go"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/domain"
)

type Sender struct {
	vapidPublicKey  string
	vapidPrivateKey string
	vapidSubject    string
}

func NewSender(vapidPublicKey, vapidPrivateKey, vapidSubject string) *Sender {
	return &Sender{
		vapidPublicKey:  vapidPublicKey,
		vapidPrivateKey: vapidPrivateKey,
		vapidSubject:    vapidSubject,
	}
}

func (s *Sender) Send(ctx context.Context, sub domain.PushSubscription, payload domain.PushPayload) {
	message, err := json.Marshal(payload)
	if err != nil {
		log.Printf("notification-worker: failed to marshal push payload: %v", err)
		return
	}

	resp, err := upstream.SendNotificationWithContext(ctx, message, &upstream.Subscription{
		Endpoint: sub.Endpoint,
		Keys: upstream.Keys{
			P256dh: sub.P256dhKey,
			Auth:   sub.AuthKey,
		},
	}, &upstream.Options{
		Subscriber:      s.vapidSubject,
		VAPIDPublicKey:  s.vapidPublicKey,
		VAPIDPrivateKey: s.vapidPrivateKey,
		TTL:             60,
	})
	if err != nil {
		log.Printf("notification-worker: failed to send web push: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("notification-worker: web push rejected, status=%d body=%s", resp.StatusCode, string(body))
		return
	}
}
