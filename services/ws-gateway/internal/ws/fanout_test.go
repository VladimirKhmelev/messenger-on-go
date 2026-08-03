package ws

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/chatclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/events"
)

type fakeMembersLister struct {
	userIDs []string
	err     error
}

func (l *fakeMembersLister) ListMembers(_ context.Context, _ string) ([]string, error) {
	return l.userIDs, l.err
}

type fakeMessageGetter struct {
	message chatclient.Message
	err     error
}

func (g *fakeMessageGetter) GetMessage(_ context.Context, _ string) (chatclient.Message, error) {
	return g.message, g.err
}

func connectSession(t *testing.T, server *httptest.Server, userID string) *websocket.Conn {
	t.Helper()

	token := issueTestAccessToken(t, userID)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + url.QueryEscape(token)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() unexpected error for user %s: %v", userID, err)
	}
	return conn
}

func TestFanout_Handle_DeliversToChatMembers(t *testing.T) {
	registry := NewRegistry()
	server := httptest.NewServer(NewHandler(testJWTSecret, &fakeChatClient{}, registry))
	defer server.Close()

	memberConn := connectSession(t, server, "member-1")
	defer func() { _ = memberConn.Close() }()

	strangerConn := connectSession(t, server, "stranger")
	defer func() { _ = strangerConn.Close() }()

	time.Sleep(50 * time.Millisecond)

	fanout := NewFanout(
		registry,
		&fakeMembersLister{userIDs: []string{"member-1"}},
		&fakeMessageGetter{message: chatclient.Message{MessageID: "m1", SenderUserID: "member-1", Text: "hello"}},
	)

	fanout.Handle(context.Background(), events.MessageCreated{MessageID: "m1", ChatID: "chat-1"})

	_ = memberConn.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := memberConn.ReadMessage()
	if err != nil {
		t.Fatalf("member did not receive fanout message: %v", err)
	}

	if !strings.Contains(string(data), `"message_received"`) || !strings.Contains(string(data), `"m1"`) {
		t.Errorf("member received unexpected payload: %s", data)
	}

	_ = strangerConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := strangerConn.ReadMessage(); err == nil {
		t.Error("stranger (not a chat member) received a fanout message, want nothing")
	}
}
