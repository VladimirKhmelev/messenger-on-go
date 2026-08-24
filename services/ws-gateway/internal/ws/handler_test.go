package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/chatclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/events"
)

const testJWTSecret = "test-secret"

type fakeChatClient struct {
	sendMessageErr error
	sentText       string
	sentChatID     string

	getHistoryMessages []chatclient.Message
	getHistoryErr      error

	presenceOnline       bool
	presenceLastSeenUnix int64
	presenceErr          error
	setOfflineErr        error

	editMessageErr         error
	deleteForAllErr        error
	deleteForMeErr         error
	editedText             string
	deletedForAllMessageID string
	deletedForMeMessageID  string

	markReadErr       error
	markReadMessageID string

	readStatusMessageID string
	readStatusErr       error
}

func (c *fakeChatClient) SendMessage(_ context.Context, _, chatID, text string) (string, error) {
	if c.sendMessageErr != nil {
		return "", c.sendMessageErr
	}
	c.sentChatID = chatID
	c.sentText = text
	return "message-1", nil
}

func (c *fakeChatClient) GetHistory(_ context.Context, _, _ string, _, _ int32) ([]chatclient.Message, error) {
	if c.getHistoryErr != nil {
		return nil, c.getHistoryErr
	}
	return c.getHistoryMessages, nil
}

func (c *fakeChatClient) GetPresence(_ context.Context, _ string) (bool, int64, error) {
	if c.presenceErr != nil {
		return false, 0, c.presenceErr
	}
	return c.presenceOnline, c.presenceLastSeenUnix, nil
}

func (c *fakeChatClient) SetOffline(_ context.Context, _ string) error {
	return c.setOfflineErr
}

func (c *fakeChatClient) EditMessage(_ context.Context, _, _, _, text string) error {
	if c.editMessageErr != nil {
		return c.editMessageErr
	}
	c.editedText = text
	return nil
}

func (c *fakeChatClient) DeleteMessageForAll(_ context.Context, _, _, messageID string) error {
	if c.deleteForAllErr != nil {
		return c.deleteForAllErr
	}
	c.deletedForAllMessageID = messageID
	return nil
}

func (c *fakeChatClient) DeleteMessageForMe(_ context.Context, _, _, messageID string) error {
	if c.deleteForMeErr != nil {
		return c.deleteForMeErr
	}
	c.deletedForMeMessageID = messageID
	return nil
}

func (c *fakeChatClient) MarkRead(_ context.Context, _, _, messageID string) error {
	if c.markReadErr != nil {
		return c.markReadErr
	}
	c.markReadMessageID = messageID
	return nil
}

func (c *fakeChatClient) GetReadStatus(_ context.Context, _, _ string) (string, error) {
	if c.readStatusErr != nil {
		return "", c.readStatusErr
	}
	return c.readStatusMessageID, nil
}

func (c *fakeChatClient) SetTyping(_ context.Context, _, _ string) error {
	return nil
}

type fakePresencePublisher struct{}

func (fakePresencePublisher) PublishPresenceChanged(_ events.PresenceChanged) error {
	return nil
}

func (fakePresencePublisher) PublishTypingChanged(_ events.TypingChanged) error {
	return nil
}

type testClaims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	Type   string `json:"type"`
}

func issueTestAccessToken(t *testing.T, userID string) string {
	t.Helper()

	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
		UserID: userID,
		Type:   "access",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

func TestHandler_MissingToken_Rejected(t *testing.T) {
	server := httptest.NewServer(NewHandler(testJWTSecret, &fakeChatClient{}, NewRegistry(), fakePresencePublisher{}, nil))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("Dial() succeeded without a token, want rejection")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("response status = %v, want %v", resp, http.StatusUnauthorized)
	}
}

func TestHandler_InvalidToken_Rejected(t *testing.T) {
	server := httptest.NewServer(NewHandler(testJWTSecret, &fakeChatClient{}, NewRegistry(), fakePresencePublisher{}, nil))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=not-a-real-token"

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("Dial() succeeded with an invalid token, want rejection")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("response status = %v, want %v", resp, http.StatusUnauthorized)
	}
}

func TestHandler_ValidToken_UpgradesConnection(t *testing.T) {
	server := httptest.NewServer(NewHandler(testJWTSecret, &fakeChatClient{}, NewRegistry(), fakePresencePublisher{}, nil))
	defer server.Close()

	token := issueTestAccessToken(t, "user-1")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + url.QueryEscape(token)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() unexpected error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("response status = %v, want %v", resp.StatusCode, http.StatusSwitchingProtocols)
	}
}

func TestHandler_SendMessage_ForwardsToChatClient(t *testing.T) {
	chat := &fakeChatClient{}
	server := httptest.NewServer(NewHandler(testJWTSecret, chat, NewRegistry(), fakePresencePublisher{}, nil))
	defer server.Close()

	token := issueTestAccessToken(t, "user-1")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + url.QueryEscape(token)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() unexpected error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	req, _ := json.Marshal(clientMessage{Type: "send_message", ChatID: "chat-1", Text: "hello"})
	if err := conn.WriteMessage(websocket.TextMessage, req); err != nil {
		t.Fatalf("WriteMessage() unexpected error: %v", err)
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() unexpected error: %v", err)
	}

	var resp serverMessage
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Type != "message_sent" || resp.MessageID != "message-1" {
		t.Errorf("response = %+v, want type=message_sent, message_id=message-1", resp)
	}
	if chat.sentChatID != "chat-1" || chat.sentText != "hello" {
		t.Errorf("chatclient received chatID=%q text=%q, want chat-1/hello", chat.sentChatID, chat.sentText)
	}
}

func TestHandler_GetHistory_ForwardsToChatClient(t *testing.T) {
	chat := &fakeChatClient{getHistoryMessages: []chatclient.Message{{MessageID: "m1", Text: "hi"}}}
	server := httptest.NewServer(NewHandler(testJWTSecret, chat, NewRegistry(), fakePresencePublisher{}, nil))
	defer server.Close()

	token := issueTestAccessToken(t, "user-1")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + url.QueryEscape(token)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() unexpected error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	req, _ := json.Marshal(clientMessage{Type: "get_history", ChatID: "chat-1", Limit: 10})
	if err := conn.WriteMessage(websocket.TextMessage, req); err != nil {
		t.Fatalf("WriteMessage() unexpected error: %v", err)
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() unexpected error: %v", err)
	}

	var resp serverMessage
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Type != "history" || len(resp.Messages) != 1 || resp.Messages[0].MessageID != "m1" {
		t.Errorf("response = %+v, want type=history with 1 message m1", resp)
	}
}

func TestHandler_EditMessage_ForwardsToChatClient(t *testing.T) {
	chat := &fakeChatClient{}
	server := httptest.NewServer(NewHandler(testJWTSecret, chat, NewRegistry(), fakePresencePublisher{}, nil))
	defer server.Close()

	token := issueTestAccessToken(t, "user-1")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + url.QueryEscape(token)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() unexpected error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	req, _ := json.Marshal(clientMessage{Type: "edit_message", ChatID: "chat-1", MessageID: "m1", Text: "edited"})
	if err := conn.WriteMessage(websocket.TextMessage, req); err != nil {
		t.Fatalf("WriteMessage() unexpected error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if chat.editedText != "edited" {
		t.Errorf("chatclient received editedText=%q, want %q", chat.editedText, "edited")
	}
}

func TestHandler_MarkRead_ForwardsToChatClient(t *testing.T) {
	chat := &fakeChatClient{}
	server := httptest.NewServer(NewHandler(testJWTSecret, chat, NewRegistry(), fakePresencePublisher{}, nil))
	defer server.Close()

	token := issueTestAccessToken(t, "user-1")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + url.QueryEscape(token)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() unexpected error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	req, _ := json.Marshal(clientMessage{Type: "mark_read", ChatID: "chat-1", MessageID: "m1"})
	if err := conn.WriteMessage(websocket.TextMessage, req); err != nil {
		t.Fatalf("WriteMessage() unexpected error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if chat.markReadMessageID != "m1" {
		t.Errorf("chatclient received markReadMessageID=%q, want %q", chat.markReadMessageID, "m1")
	}
}
