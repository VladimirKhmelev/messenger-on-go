FROM golang:1.26.2 AS builder

WORKDIR /src

COPY proto/gen proto/gen
COPY services/chat-service services/chat-service

WORKDIR /src/services/chat-service
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.34

FROM alpine:3.20

COPY --from=builder /out/server /server
COPY --from=builder /go/bin/grpc-health-probe /grpc-health-probe

HEALTHCHECK --interval=5s --timeout=5s --retries=5 \
    CMD ["/grpc-health-probe", "-addr=:50051"]

ENTRYPOINT ["/server"]
