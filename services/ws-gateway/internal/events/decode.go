package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
)

func decode[T any](data []byte, key func(T) string, handle func(context.Context, T) error) (string, func(context.Context) error, error) {
	var event T
	if err := json.Unmarshal(data, &event); err != nil {
		return "", nil, err
	}
	k := key(event)
	if k == "" || handle == nil {
		return "", nil, fmt.Errorf("missing routing key or handler")
	}
	return k, func(ctx context.Context) error { return handle(ctx, event) }, nil
}

func decodeEvent(subject string, data []byte, h Handlers) (string, func(context.Context) error, error) {
	switch subject {
	case subjectMsg:
		return decode(data, func(e domain.MessageCreated) string {
			if e.ChatID == "" {
				return ""
			}
			return "chat:" + e.ChatID
		}, h.OnMessageCreated)
	case subjectMsgEdit, subjectMsgDelete:
		return decode(data, func(e domain.MessageUpdated) string {
			if e.ChatID == "" {
				return ""
			}
			return "chat:" + e.ChatID
		}, h.OnMessageUpdated)
	case subjectMsgRead:
		return decode(data, func(e domain.MessageRead) string {
			if e.ChatID == "" {
				return ""
			}
			return "chat:" + e.ChatID
		}, h.OnMessageRead)
	case subjectChatDelete:
		return decode(data, func(e domain.ChatDeleted) string {
			if e.ChatID == "" {
				return ""
			}
			return "chat:" + e.ChatID
		}, h.OnChatDeleted)
	case subjectNotify:
		return decode(data, func(e domain.NotifyPush) string {
			if e.UserID == "" {
				return ""
			}
			return "notify:" + e.UserID
		}, h.OnNotifyPush)
	case subjectProfileUpdated:
		return decode(data, func(e domain.ProfileUpdated) string {
			if e.UserID == "" {
				return ""
			}
			return "user:" + e.UserID
		}, h.OnProfileUpdated)
	case subjectPresence:
		return decode(data, func(e domain.PresenceChanged) string {
			if e.UserID == "" {
				return ""
			}
			return "user:" + e.UserID
		}, h.OnPresenceChanged)
	case subjectTyping:
		return decode(data, func(e domain.TypingChanged) string {
			if e.ChatID == "" {
				return ""
			}
			return "typing:" + e.ChatID
		}, h.OnTypingChanged)
	default:
		return "", nil, fmt.Errorf("unknown event subject %q", subject)
	}
}
