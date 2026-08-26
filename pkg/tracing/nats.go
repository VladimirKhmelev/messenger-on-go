package tracing

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var natsTracer = otel.Tracer("nats")

type natsHeaderCarrier nats.Header

func (c natsHeaderCarrier) Get(key string) string {
	values := nats.Header(c)[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (c natsHeaderCarrier) Set(key, value string) {
	nats.Header(c).Set(key, value)
}

func (c natsHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func InjectNATSHeader(ctx context.Context, header nats.Header) {
	otel.GetTextMapPropagator().Inject(ctx, natsHeaderCarrier(header))
}

func ExtractNATSHeader(ctx context.Context, header nats.Header) context.Context {
	if header == nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, natsHeaderCarrier(header))
}

func StartPublishSpan(ctx context.Context, subject string) (context.Context, nats.Header, trace.Span) {
	ctx, span := natsTracer.Start(ctx, subject+" send",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
		),
	)
	header := nats.Header{}
	InjectNATSHeader(ctx, header)
	return ctx, header, span
}

func StartConsumeSpan(ctx context.Context, subject string, header nats.Header) (context.Context, trace.Span) {
	ctx = ExtractNATSHeader(ctx, header)
	return natsTracer.Start(ctx, subject+" process",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
		),
	)
}
