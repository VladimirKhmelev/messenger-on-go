package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/chatclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway/internal/ws"
)

func main() {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}

	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("ws-gateway: JWT_SECRET is required")
	}

	internalSecret := os.Getenv("INTERNAL_SECRET")
	if internalSecret == "" {
		log.Fatal("ws-gateway: INTERNAL_SECRET is required")
	}

	chatServiceAddr := os.Getenv("CHAT_SERVICE_ADDR")
	if chatServiceAddr == "" {
		log.Fatal("ws-gateway: CHAT_SERVICE_ADDR is required")
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		log.Fatal("ws-gateway: NATS_URL is required")
	}

	var allowedOrigins []string
	if raw := os.Getenv("ALLOWED_ORIGINS"); raw != "" {
		allowedOrigins = strings.Split(raw, ",")
	}

	chatClient, err := chatclient.Dial(chatServiceAddr, internalSecret)
	if err != nil {
		log.Fatalf("ws-gateway: failed to dial chat-service: %v", err)
	}
	defer func() { _ = chatClient.Close() }()

	presencePublisher, err := events.ConnectPresencePublisher(natsURL)
	if err != nil {
		log.Fatalf("ws-gateway: failed to connect presence publisher: %v", err)
	}
	defer presencePublisher.Close()

	registry := ws.NewRegistry()
	fanout := ws.NewFanout(registry, chatClient, chatClient, chatClient)

	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	defer cancelConsumer()

	go func() {
		handlers := events.Handlers{
			OnMessageCreated:  fanout.HandleMessageCreated,
			OnMessageUpdated:  fanout.HandleMessageUpdated,
			OnMessageRead:     fanout.HandleMessageRead,
			OnNotifyPush:      fanout.HandleNotifyPush,
			OnPresenceChanged: fanout.HandlePresenceChanged,
			OnProfileUpdated:  fanout.HandleProfileUpdated,
			OnTypingChanged:   fanout.HandleTypingChanged,
		}
		if err := events.Consume(consumerCtx, natsURL, handlers); err != nil {
			log.Fatalf("ws-gateway: NATS consumer failed: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("ws-gateway: failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		fmt.Printf("ws-gateway: grpc listening on :%s\n", port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("ws-gateway: grpc serve failed: %v", err)
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/ws", ws.NewHandler(jwtSecret, chatClient, registry, presencePublisher, allowedOrigins))

	httpServer := &http.Server{
		Addr:    ":" + httpPort,
		Handler: mux,
	}

	go func() {
		fmt.Printf("ws-gateway: http listening on :%s\n", httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ws-gateway: http serve failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("ws-gateway: shutting down")
	grpcServer.GracefulStop()
	_ = httpServer.Close()
}
