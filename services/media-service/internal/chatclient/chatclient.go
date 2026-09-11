package chatclient

import (
	"context"
	"slices"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	chatv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/chat/v1"
)

type Client struct {
	conn           *grpc.ClientConn
	chat           chatv1.ChatServiceClient
	internalSecret string
}

func Dial(addr, internalSecret string) (*Client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(metrics.UnaryClientInterceptor("media-service")),
	)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, chat: chatv1.NewChatServiceClient(conn), internalSecret: internalSecret}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) IsMember(ctx context.Context, chatID, userID string) (bool, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "x-internal-secret", c.internalSecret)

	resp, err := c.chat.ListMembers(ctx, &chatv1.ListMembersRequest{ChatId: chatID})
	if err != nil {
		return false, err
	}
	return slices.Contains(resp.GetUserIds(), userID), nil
}
