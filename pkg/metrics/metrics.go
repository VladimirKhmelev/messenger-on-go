package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	metrics.WritePrometheus(w, true)
}

func UnaryServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		observe(serviceName, info.FullMethod, err, time.Since(start))
		return resp, err
	}
}

func UnaryClientInterceptor(serviceName string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		observe(serviceName, method, err, time.Since(start))
		return err
	}
}

func observe(serviceName, method string, err error, elapsed time.Duration) {
	label := fmt.Sprintf(`grpc_request_duration_seconds{service=%q, method=%q, code=%q}`,
		serviceName, method, status.Code(err).String())
	metrics.GetOrCreateHistogram(label).Update(elapsed.Seconds())
}

var FailedLoginsTotal = metrics.NewCounter(`auth_failed_logins_total`)

var MessagesSentTotal = metrics.NewCounter(`chat_messages_sent_total`)

var NotifyPushTotal = metrics.NewCounter(`notification_push_total`)

var NATSConsumeErrorsTotal = metrics.NewCounter(`nats_consume_errors_total`)

var wsConnectionsActive atomic.Int64

func init() {
	metrics.NewGauge(`ws_connections_active`, func() float64 {
		return float64(wsConnectionsActive.Load())
	})
}

func WSConnectionOpened() { wsConnectionsActive.Add(1) }
func WSConnectionClosed() { wsConnectionsActive.Add(-1) }
