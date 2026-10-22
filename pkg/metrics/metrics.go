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

type Option func(*options)

type options struct {
	transport string
}

// Transport labels the calls with the transport they go over, "grpc" by
// default and "quic" for the calls over quic-rpc-go.
func Transport(name string) Option {
	return func(o *options) { o.transport = name }
}

func newOptions(opts []Option) options {
	o := options{transport: "grpc"}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func UnaryServerInterceptor(serviceName string, opts ...Option) grpc.UnaryServerInterceptor {
	o := newOptions(opts)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		observe(serviceName, o.transport, info.FullMethod, err, time.Since(start))
		return resp, err
	}
}

func UnaryClientInterceptor(serviceName string, opts ...Option) grpc.UnaryClientInterceptor {
	o := newOptions(opts)
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		observe(serviceName, o.transport, method, err, time.Since(start))
		return err
	}
}

func observe(serviceName, transport, method string, err error, elapsed time.Duration) {
	label := fmt.Sprintf(`grpc_request_duration_seconds{service=%q, transport=%q, method=%q, code=%q}`,
		serviceName, transport, method, status.Code(err).String())
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
