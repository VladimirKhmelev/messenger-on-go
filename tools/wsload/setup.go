package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// The load goes straight to the stack, past any proxy of the environment,
// so that only the stack is measured.
var (
	httpClient = &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{}}
	dialer     = &websocket.Dialer{HandshakeTimeout: 10 * time.Second}
)

// user is a made-up user with a chat with itself.
type user struct{ id, token, chatID string }

// newID returns a random UUID, as the IDs of the users are.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])  // it never fails
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // the variant of RFC 9562
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// newUser makes up a user whose token expires at expires.
func newUser(ctx context.Context, chatURL, secret string, expires time.Time) (user, error) {
	u := user{id: newID()}
	var err error
	if u.token, err = signToken(secret, u.id, expires); err != nil {
		return user{}, err
	}
	if u.chatID, err = createChat(ctx, chatURL, u); err != nil {
		return user{}, err
	}
	return u, nil
}

// signToken signs an access token as auth-service does.
func signToken(secret, userID string, expires time.Time) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID, "type": "access", "iat": time.Now().Unix(), "exp": expires.Unix(),
	}).SignedString([]byte(secret))
}

// createChat creates the chat of a user with itself: chat-service asks
// auth-service about the other user of a chat, but not about the requester.
// The chat keys of end-to-end encryption are placeholders: nobody reads the
// messages.
func createChat(ctx context.Context, chatURL string, u user) (string, error) {
	body, err := json.Marshal(map[string]any{
		"target_user_id":         u.id,
		"encrypted_chat_key":     map[string]string{u.id: "wsload"},
		"wrapped_for_public_key": map[string]string{u.id: "wsload"},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL+"/v1/chats", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+u.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create a chat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("create a chat: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create a chat: %s: %s", resp.Status, bytes.TrimSpace(data))
	}
	var created struct {
		ChatID string `json:"chatId"`
	}
	if err := json.Unmarshal(data, &created); err != nil || created.ChatID == "" {
		return "", fmt.Errorf("create a chat: no chat ID in %s", bytes.TrimSpace(data))
	}
	return created.ChatID, nil
}

// dial connects a user to a gateway.
func dial(ctx context.Context, gatewayURL, token string) (*websocket.Conn, error) {
	u, err := url.Parse(gatewayURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	ws, resp, err := dialer.DialContext(ctx, u.String(), nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return ws, nil
}
