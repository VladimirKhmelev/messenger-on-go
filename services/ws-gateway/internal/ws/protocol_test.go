package ws

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/hub"
)

func TestWireMessageJSONKeys(t *testing.T) {
	wire := toWireMessage(domain.Message{
		MessageID:     "m1",
		SenderUserID:  "u1",
		Text:          "hi",
		CreatedAtUnix: 10,
		EditedAtUnix:  20,
		Deleted:       true,
	})

	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	want := map[string]any{
		"message_id":      "m1",
		"sender_user_id":  "u1",
		"text":            "hi",
		"created_at_unix": float64(10),
		"edited_at_unix":  float64(20),
		"deleted":         true,
	}
	if len(got) != len(want) {
		t.Fatalf("got keys %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
}

func TestToServerMessage(t *testing.T) {
	newText := "edited"
	tests := []struct {
		name         string
		notification any
		want         serverMessage
	}{
		{"message received", hub.MessageReceived{ChatID: "c1", Message: domain.Message{MessageID: "m1", Text: "hi"}},
			serverMessage{Type: "message_received", ChatID: "c1", Message: &wireMessage{MessageID: "m1", Text: "hi"}}},
		{"message updated", domain.MessageUpdated{ChatID: "c1", MessageID: "m1", NewBody: &newText, Deleted: false},
			serverMessage{Type: "message_updated", ChatID: "c1", MessageID: "m1", NewText: &newText}},
		{"message read", domain.MessageRead{ChatID: "c1", UserID: "u1", MessageID: "m1"},
			serverMessage{Type: "read_status", ChatID: "c1", PeerUserID: "u1", LastReadMessageID: "m1"}},
		{"notify push", domain.NotifyPush{UserID: "u1", ChatID: "c1", MessageID: "m1"},
			serverMessage{Type: "notify_push", ChatID: "c1", MessageID: "m1"}},
		{"presence changed", domain.PresenceChanged{UserID: "u1", Online: true, LastSeenUnix: 5},
			serverMessage{Type: "presence_changed", PeerUserID: "u1", Online: true, LastSeenUnix: 5}},
		{"typing changed", domain.TypingChanged{ChatID: "c1", UserID: "u1"},
			serverMessage{Type: "typing_changed", ChatID: "c1", PeerUserID: "u1"}},
		{"chat deleted", domain.ChatDeleted{ChatID: "c1", MemberUserIDs: []string{"u1"}},
			serverMessage{Type: "chat_deleted", ChatID: "c1"}},
		{"profile updated", domain.ProfileUpdated{UserID: "u1", Tag: "bob", DisplayName: "Bob"},
			serverMessage{Type: "profile_updated", PeerUserID: "u1", PeerTag: "bob", PeerDisplayName: "Bob"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toServerMessage(tt.notification)
			if !ok {
				t.Fatalf("toServerMessage(%T) not mapped", tt.notification)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("toServerMessage() = %+v, want %+v", got, tt.want)
			}
		})
	}

	if _, ok := toServerMessage(struct{}{}); ok {
		t.Fatal("toServerMessage(unknown) mapped, want dropped")
	}
}
