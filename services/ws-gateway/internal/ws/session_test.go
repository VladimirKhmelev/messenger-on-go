package ws

import (
	"encoding/json"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
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
