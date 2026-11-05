package chatclient

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/pkg/quictransport"
	chatv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/chat/v1"
)

type Client struct {
	conn           conn
	chat           chatv1.ChatServiceClient
	internalSecret string
}

// conn is the connection to chat-service, over gRPC or QUIC.
type conn interface {
	grpc.ClientConnInterface
	Close() error
}

func Dial(addr, internalSecret string) (*Client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(metrics.UnaryClientInterceptor("notification-worker")),
	)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, chat: chatv1.NewChatServiceClient(conn), internalSecret: internalSecret}, nil
}

// DialQUIC is Dial over QUIC; caFile holds the CA certificate used to
// verify chat-service's certificate.
func DialQUIC(addr, internalSecret, caFile string) (*Client, error) {
	conn, err := quictransport.Dial(quictransport.ClientConfig{
		Addr:   addr,
		CAFile: caFile,
		Interceptors: []grpc.UnaryClientInterceptor{
			metrics.UnaryClientInterceptor("notification-worker", metrics.Transport(quictransport.Name)),
		},
	})
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, chat: chatv1.NewChatServiceClient(conn), internalSecret: internalSecret}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) ListMembers(ctx context.Context, chatID string) ([]string, error) {
	ctx = c.withInternalSecret(ctx)

	resp, err := c.chat.ListMembers(ctx, &chatv1.ListMembersRequest{ChatId: chatID})
	if err != nil {
		return nil, err
	}
	return resp.GetUserIds(), nil
}

func (c *Client) IsOnline(ctx context.Context, userID string) (bool, error) {
	ctx = c.withInternalSecret(ctx)

	resp, err := c.chat.GetPresence(ctx, &chatv1.GetPresenceRequest{UserId: userID})
	if err != nil {
		return false, err
	}
	return resp.GetOnline(), nil
}

func (c *Client) withInternalSecret(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-internal-secret", c.internalSecret)
}
