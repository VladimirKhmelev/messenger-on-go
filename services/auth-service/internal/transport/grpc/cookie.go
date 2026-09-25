package grpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
)

var errNoCookie = errors.New("refresh_token cookie not present")

const refreshCookieName = "refresh_token"

func setRefreshCookie(ctx context.Context, token string, secure bool) error {
	cookie := fmt.Sprintf(
		"%s=%s; Path=/; Max-Age=%d; HttpOnly; SameSite=Lax%s",
		refreshCookieName, token, int(jwtutil.RefreshTokenTTL.Seconds()), secureAttr(secure),
	)
	return grpc.SetHeader(ctx, metadata.Pairs("set-cookie", cookie))
}

func clearRefreshCookie(ctx context.Context, secure bool) error {
	cookie := fmt.Sprintf("%s=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax%s", refreshCookieName, secureAttr(secure))
	return grpc.SetHeader(ctx, metadata.Pairs("set-cookie", cookie))
}

func secureAttr(secure bool) string {
	if secure {
		return "; Secure"
	}
	return ""
}

func refreshTokenFromCookie(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errNoCookie
	}

	values := md.Get("grpcgateway-cookie")
	if len(values) == 0 {
		return "", errNoCookie
	}

	header := http.Header{}
	header.Add("Cookie", values[0])
	request := http.Request{Header: header}

	cookie, err := request.Cookie(refreshCookieName)
	if err != nil {
		return "", errNoCookie
	}

	return cookie.Value, nil
}
