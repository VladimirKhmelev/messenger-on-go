FROM golang:1.26.2 AS builder

ENV GODEBUG=netdns=go+4

WORKDIR /src

COPY proto/gen proto/gen
COPY pkg/tracing pkg/tracing
COPY services/notification-worker services/notification-worker

WORKDIR /src/services/notification-worker
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/worker
RUN CGO_ENABLED=0 go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.34

FROM alpine:3.20

RUN addgroup -S app && adduser -S -G app app

COPY --from=builder /out/worker /worker
COPY --from=builder /go/bin/grpc-health-probe /grpc-health-probe

USER app

HEALTHCHECK --interval=5s --timeout=5s --retries=5 \
    CMD ["/grpc-health-probe", "-addr=:50051"]

ENTRYPOINT ["/worker"]
