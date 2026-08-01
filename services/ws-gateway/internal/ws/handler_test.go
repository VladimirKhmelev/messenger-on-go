package ws

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

const testJWTSecret = "test-secret"

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
	server := httptest.NewServer(NewHandler(testJWTSecret))
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
	server := httptest.NewServer(NewHandler(testJWTSecret))
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
	server := httptest.NewServer(NewHandler(testJWTSecret))
	defer server.Close()

	token := issueTestAccessToken(t, "user-1")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=" + url.QueryEscape(token)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() unexpected error: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("response status = %v, want %v", resp.StatusCode, http.StatusSwitchingProtocols)
	}
}
