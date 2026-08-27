package authclient

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
)

type Client struct {
	conn *grpc.ClientConn
	auth authv1.AuthServiceClient
}

func Dial(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(metrics.UnaryClientInterceptor("chat-service")),
	)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, auth: authv1.NewAuthServiceClient(conn)}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) UserExists(ctx context.Context, bearerToken, userID string) (bool, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+bearerToken)

	_, err := c.auth.GetUserByID(ctx, &authv1.GetUserByIDRequest{UserId: userID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
