package authclient

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/pkg/quictransport"
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/domain"
)

type Client struct {
	conn           conn
	auth           authv1.AuthServiceClient
	internalSecret string
}

// conn is the connection to auth-service, over gRPC or QUIC.
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
	return &Client{conn: conn, auth: authv1.NewAuthServiceClient(conn), internalSecret: internalSecret}, nil
}

// DialQUIC is Dial over QUIC; caFile holds the CA certificate used to
// verify auth-service's certificate.
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
	return &Client{conn: conn, auth: authv1.NewAuthServiceClient(conn), internalSecret: internalSecret}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) ListPushSubscriptions(ctx context.Context, userID string) ([]domain.PushSubscription, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "x-internal-secret", c.internalSecret)

	resp, err := c.auth.ListPushSubscriptions(ctx, &authv1.ListPushSubscriptionsRequest{UserId: userID})
	if err != nil {
		return nil, err
	}

	subs := make([]domain.PushSubscription, 0, len(resp.GetSubscriptions()))
	for _, s := range resp.GetSubscriptions() {
		subs = append(subs, domain.PushSubscription{
			Endpoint:  s.GetEndpoint(),
			P256dhKey: s.GetP256DhKey(),
			AuthKey:   s.GetAuthKey(),
		})
	}
	return subs, nil
}
