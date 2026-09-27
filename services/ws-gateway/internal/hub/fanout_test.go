package hub

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/domain"
)

type fakeClient struct {
	got []any
}

func (c *fakeClient) Deliver(n any) { c.got = append(c.got, n) }

type fakeMembers struct {
	ids []string
	err error
}

func (f fakeMembers) ListMembers(context.Context, string) ([]string, error) { return f.ids, f.err }

type fakeMessages struct {
	msg domain.Message
	err error
}

func (f fakeMessages) GetMessage(context.Context, string) (domain.Message, error) {
	return f.msg, f.err
}

type fakeContacts struct {
	ids []string
	err error
}

func (f fakeContacts) ListContacts(context.Context, string) ([]string, error) { return f.ids, f.err }

// connect registers one fake client per user and returns them by user ID.
func connect(r *Registry, userIDs ...string) map[string]*fakeClient {
	clients := make(map[string]*fakeClient, len(userIDs))
	for _, id := range userIDs {
		c := &fakeClient{}
		r.Add(id, c)
		clients[id] = c
	}
	return clients
}

func TestFanout_MessageCreated_DeliversFullMessageToMembersOnly(t *testing.T) {
	r := NewRegistry()
	clients := connect(r, "alice", "bob", "stranger")
	msg := domain.Message{MessageID: "m1", SenderUserID: "alice", Text: "hi"}
	f := NewFanout(r, fakeMembers{ids: []string{"alice", "bob"}}, fakeMessages{msg: msg}, fakeContacts{})

	f.HandleMessageCreated(context.Background(), domain.MessageCreated{ChatID: "c1", MessageID: "m1"})

	want := []any{MessageReceived{ChatID: "c1", Message: msg}}
	for _, id := range []string{"alice", "bob"} {
		if !reflect.DeepEqual(clients[id].got, want) {
			t.Errorf("%s got %v, want %v", id, clients[id].got, want)
		}
	}
	if len(clients["stranger"].got) != 0 {
		t.Errorf("stranger got %v, want nothing", clients["stranger"].got)
	}
}

func TestFanout_MessageCreated_NothingSentWhenMessageFetchFails(t *testing.T) {
	r := NewRegistry()
	clients := connect(r, "alice")
	f := NewFanout(r, fakeMembers{ids: []string{"alice"}}, fakeMessages{err: errors.New("chat-service down")}, fakeContacts{})

	f.HandleMessageCreated(context.Background(), domain.MessageCreated{ChatID: "c1", MessageID: "m1"})

	if len(clients["alice"].got) != 0 {
		t.Errorf("alice got %v, want nothing", clients["alice"].got)
	}
}

// The actor of a read receipt or typing indicator must not get it echoed back.
func TestFanout_SkipsActor(t *testing.T) {
	tests := []struct {
		name string
		send func(f *Fanout)
	}{
		{"message read", func(f *Fanout) {
			f.HandleMessageRead(context.Background(), domain.MessageRead{ChatID: "c1", UserID: "alice", MessageID: "m1"})
		}},
		{"typing changed", func(f *Fanout) {
			f.HandleTypingChanged(context.Background(), domain.TypingChanged{ChatID: "c1", UserID: "alice"})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			clients := connect(r, "alice", "bob")
			tt.send(NewFanout(r, fakeMembers{ids: []string{"alice", "bob"}}, fakeMessages{}, fakeContacts{}))

			if len(clients["alice"].got) != 0 {
				t.Errorf("actor alice got %v, want nothing", clients["alice"].got)
			}
			if len(clients["bob"].got) != 1 {
				t.Errorf("bob got %d notifications, want 1", len(clients["bob"].got))
			}
		})
	}
}

func TestFanout_PresenceChanged_GoesToContacts(t *testing.T) {
	r := NewRegistry()
	clients := connect(r, "contact", "stranger")
	f := NewFanout(r, fakeMembers{}, fakeMessages{}, fakeContacts{ids: []string{"contact"}})
	event := domain.PresenceChanged{UserID: "alice", Online: true}

	f.HandlePresenceChanged(context.Background(), event)

	if !reflect.DeepEqual(clients["contact"].got, []any{event}) {
		t.Errorf("contact got %v, want %v", clients["contact"].got, []any{event})
	}
	if len(clients["stranger"].got) != 0 {
		t.Errorf("stranger got %v, want nothing", clients["stranger"].got)
	}
}

func TestFanout_NotifyPush_GoesOnlyToTarget(t *testing.T) {
	r := NewRegistry()
	clients := connect(r, "alice", "bob")
	f := NewFanout(r, fakeMembers{}, fakeMessages{}, fakeContacts{})

	f.HandleNotifyPush(context.Background(), domain.NotifyPush{UserID: "bob", ChatID: "c1", MessageID: "m1"})

	if len(clients["alice"].got) != 0 || len(clients["bob"].got) != 1 {
		t.Errorf("alice got %d, bob got %d, want 0 and 1", len(clients["alice"].got), len(clients["bob"].got))
	}
}

func TestRegistry_BroadcastReachesEveryConnectionOfUser(t *testing.T) {
	r := NewRegistry()
	tab1, tab2 := &fakeClient{}, &fakeClient{}
	r.Add("alice", tab1)
	r.Add("alice", tab2)

	r.Broadcast("alice", "ping")

	if len(tab1.got) != 1 || len(tab2.got) != 1 {
		t.Errorf("tabs got %d and %d, want 1 each", len(tab1.got), len(tab2.got))
	}
}

func TestRegistry_RemoveReportsOtherConnections(t *testing.T) {
	r := NewRegistry()
	tab1, tab2 := &fakeClient{}, &fakeClient{}
	r.Add("alice", tab1)
	r.Add("alice", tab2)

	if !r.Remove("alice", tab1) {
		t.Error("Remove(first tab) = false, want true: second tab is still open")
	}
	if r.Remove("alice", tab2) {
		t.Error("Remove(last tab) = true, want false")
	}
}
