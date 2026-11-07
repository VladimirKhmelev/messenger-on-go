package events

import (
	"context"
	"errors"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
)

func TestDecodeRoutesAndPropagatesErrors(t *testing.T) {
	failure := errors.New("delivery failed")
	h := Handlers{
		OnMessageCreated:  func(context.Context, domain.MessageCreated) error { return failure },
		OnMessageUpdated:  func(context.Context, domain.MessageUpdated) error { return failure },
		OnMessageRead:     func(context.Context, domain.MessageRead) error { return failure },
		OnChatDeleted:     func(context.Context, domain.ChatDeleted) error { return failure },
		OnNotifyPush:      func(context.Context, domain.NotifyPush) error { return failure },
		OnPresenceChanged: func(context.Context, domain.PresenceChanged) error { return failure },
		OnProfileUpdated:  func(context.Context, domain.ProfileUpdated) error { return failure },
		OnTypingChanged:   func(context.Context, domain.TypingChanged) error { return failure },
	}
	for _, tc := range []struct{ subject, key string }{
		{subjectMsg, "chat:c"}, {subjectMsgEdit, "chat:c"}, {subjectMsgDelete, "chat:c"}, {subjectMsgRead, "chat:c"}, {subjectChatDelete, "chat:c"},
		{subjectNotify, "notify:u"}, {subjectPresence, "user:u"}, {subjectProfileUpdated, "user:u"}, {subjectTyping, "typing:c"},
	} {
		t.Run(tc.subject, func(t *testing.T) {
			key, run, err := decodeEvent(tc.subject, []byte(`{"chat_id":"c","user_id":"u"}`), h)
			if err != nil || key != tc.key {
				t.Fatalf("key = %s, error = %v", key, err)
			}
			if err := run(t.Context()); !errors.Is(err, failure) {
				t.Fatalf("handler error = %v", err)
			}
			if _, _, err := decodeEvent(tc.subject, []byte(`{}`), h); err == nil {
				t.Fatal("missing routing key accepted")
			}
		})
	}
	for _, tc := range []struct {
		subject, data string
		h             Handlers
	}{
		{subjectMsg, `{`, h}, {"unknown", `{}`, h}, {subjectMsg, `{"chat_id":"c"}`, Handlers{}},
	} {
		if _, _, err := decodeEvent(tc.subject, []byte(tc.data), tc.h); err == nil {
			t.Fatal("invalid event accepted")
		}
	}
}
