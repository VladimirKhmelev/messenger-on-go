package grpc

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeTransportStream struct {
	method string
	header metadata.MD
}

func (s *fakeTransportStream) Method() string { return s.method }

func (s *fakeTransportStream) SetHeader(md metadata.MD) error {
	s.header = metadata.Join(s.header, md)
	return nil
}

func (s *fakeTransportStream) SendHeader(md metadata.MD) error { return s.SetHeader(md) }
func (s *fakeTransportStream) SetTrailer(metadata.MD) error    { return nil }

func TestSetRefreshCookie_NotSecure(t *testing.T) {
	stream := &fakeTransportStream{method: "/auth.v1.AuthService/Login"}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)

	if err := setRefreshCookie(ctx, "the-token", false); err != nil {
		t.Fatalf("setRefreshCookie() unexpected error: %v", err)
	}

	values := stream.header.Get("set-cookie")
	if len(values) != 1 {
		t.Fatalf("expected 1 set-cookie header, got %d", len(values))
	}
	cookie := values[0]

	if !strings.Contains(cookie, "refresh_token=the-token") {
		t.Errorf("cookie = %q, want it to contain the token", cookie)
	}
	if !strings.Contains(cookie, "HttpOnly") {
		t.Errorf("cookie = %q, want HttpOnly", cookie)
	}
	if strings.Contains(cookie, "Secure") {
		t.Errorf("cookie = %q, want no Secure attribute when secure=false", cookie)
	}
}

func TestSetRefreshCookie_Secure(t *testing.T) {
	stream := &fakeTransportStream{method: "/auth.v1.AuthService/Login"}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)

	if err := setRefreshCookie(ctx, "the-token", true); err != nil {
		t.Fatalf("setRefreshCookie() unexpected error: %v", err)
	}

	cookie := stream.header.Get("set-cookie")[0]
	if !strings.Contains(cookie, "; Secure") {
		t.Errorf("cookie = %q, want Secure attribute when secure=true", cookie)
	}
}

func TestClearRefreshCookie(t *testing.T) {
	stream := &fakeTransportStream{method: "/auth.v1.AuthService/Logout"}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)

	if err := clearRefreshCookie(ctx, false); err != nil {
		t.Fatalf("clearRefreshCookie() unexpected error: %v", err)
	}

	cookie := stream.header.Get("set-cookie")[0]
	if !strings.Contains(cookie, "refresh_token=;") {
		t.Errorf("cookie = %q, want empty value", cookie)
	}
	if !strings.Contains(cookie, "Max-Age=0") {
		t.Errorf("cookie = %q, want Max-Age=0", cookie)
	}
}

func TestRefreshTokenFromCookie_Present(t *testing.T) {
	md := metadata.New(map[string]string{"grpcgateway-cookie": "other=1; refresh_token=abc123; more=2"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	token, err := refreshTokenFromCookie(ctx)
	if err != nil {
		t.Fatalf("refreshTokenFromCookie() unexpected error: %v", err)
	}
	if token != "abc123" {
		t.Errorf("refreshTokenFromCookie() = %q, want %q", token, "abc123")
	}
}

func TestRefreshTokenFromCookie_Missing(t *testing.T) {
	_, err := refreshTokenFromCookie(context.Background())
	if err != errNoCookie {
		t.Errorf("refreshTokenFromCookie() error = %v, want %v", err, errNoCookie)
	}
}

func TestRefreshTokenFromCookie_NoMatchingCookie(t *testing.T) {
	md := metadata.New(map[string]string{"grpcgateway-cookie": "other=1"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := refreshTokenFromCookie(ctx)
	if err != errNoCookie {
		t.Errorf("refreshTokenFromCookie() error = %v, want %v", err, errNoCookie)
	}
}
