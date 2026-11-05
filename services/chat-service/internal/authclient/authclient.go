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
	"github.com/VladimirKhmelev/messenger-on-go/pkg/quictransport"
	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
)

type Client struct {
	conn conn
	auth authv1.AuthServiceClient
}

// conn is the connection to auth-service, over gRPC or QUIC.
type conn interface {
	grpc.ClientConnInterface
	Close() error
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

// DialQUIC is Dial over QUIC; caFile holds the CA certificate used to
// verify auth-service's certificate.
func DialQUIC(addr, caFile string) (*Client, error) {
	conn, err := quictransport.Dial(quictransport.ClientConfig{
		Addr:   addr,
		CAFile: caFile,
		Interceptors: []grpc.UnaryClientInterceptor{
			metrics.UnaryClientInterceptor("chat-service", metrics.Transport(quictransport.Name)),
		},
	})
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
