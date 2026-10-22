package metrics

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/VictoriaMetrics/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestTransportLabel: calls are labeled with their transport, gRPC unless
// an interceptor is told otherwise.
func TestTransportLabel(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/test.v1.Test/Call"}
	ok := func(context.Context, any) (any, error) { return nil, nil }
	if _, err := UnaryServerInterceptor("test-server")(context.Background(), nil, info, ok); err != nil {
		t.Fatal(err)
	}
	if _, err := UnaryServerInterceptor("test-server", Transport("quic"))(context.Background(), nil, info, ok); err != nil {
		t.Fatal(err)
	}
	fail := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		return status.Error(codes.NotFound, "gone")
	}
	_ = UnaryClientInterceptor("test-client", Transport("quic"))(context.Background(), "/test.v1.Test/Call", nil, nil, nil, fail)

	var b bytes.Buffer
	metrics.WritePrometheus(&b, false)
	out := b.String()
	for _, want := range []string{
		`grpc_request_duration_seconds_count{service="test-server", transport="grpc", method="/test.v1.Test/Call", code="OK"} 1`,
		`grpc_request_duration_seconds_count{service="test-server", transport="quic", method="/test.v1.Test/Call", code="OK"} 1`,
		`grpc_request_duration_seconds_count{service="test-client", transport="quic", method="/test.v1.Test/Call", code="NotFound"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}
}
