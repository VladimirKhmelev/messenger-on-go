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
)

const testJWTSecret = "test-secret"

type fakeChatClient struct {
	sendMessageErr error
	sentText       string
	sentChatID     string

	getHistoryMessages []chatclient.Message
	getHistoryErr      error
}

func (c *fakeChatClient) SendMessage(_ context.Context, _, chatID, text string) (string, error) {
	if c.sendMessageErr != nil {
		return "", c.sendMessageErr
	}
	c.sentChatID = chatID
	c.sentText = text
	return "message-1", nil
}

func (c *fakeChatClient) GetHistory(_ context.Context, _, _ string, _ int32) ([]chatclient.Message, error) {
	if c.getHistoryErr != nil {
		return nil, c.getHistoryErr
	}
	return c.getHistoryMessages, nil
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
	server := httptest.NewServer(NewHandler(testJWTSecret, &fakeChatClient{}))
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
	server := httptest.NewServer(NewHandler(testJWTSecret, &fakeChatClient{}))
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
	server := httptest.NewServer(NewHandler(testJWTSecret, &fakeChatClient{}))
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
	server := httptest.NewServer(NewHandler(testJWTSecret, chat))
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
