package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	chatv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/chat/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/authclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/repository"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/service"
	transportgrpc "github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/transport/grpc"
)

func main() {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("chat-service: POSTGRES_DSN is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("chat-service: JWT_SECRET is required")
	}

	authServiceAddr := os.Getenv("AUTH_SERVICE_ADDR")
	if authServiceAddr == "" {
		log.Fatal("chat-service: AUTH_SERVICE_ADDR is required")
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		log.Fatal("chat-service: NATS_URL is required")
	}

	chatRepo, err := repository.NewPostgresChatRepository(dsn)
	if err != nil {
		log.Fatalf("chat-service: failed to connect to postgres: %v", err)
	}

	if err := chatRepo.Migrate(); err != nil {
		log.Fatalf("chat-service: failed to run migrations: %v", err)
	}

	authClient, err := authclient.Dial(authServiceAddr)
	if err != nil {
		log.Fatalf("chat-service: failed to dial auth-service: %v", err)
	}
	defer func() { _ = authClient.Close() }()

	eventPublisher, err := events.Connect(context.Background(), natsURL)
	if err != nil {
		log.Fatalf("chat-service: failed to connect to NATS: %v", err)
	}

	chatService := service.NewChatService(chatRepo, authClient, eventPublisher)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("chat-service: failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(transportgrpc.AuthInterceptor(jwtSecret)),
	)

	chatv1.RegisterChatServiceServer(grpcServer, transportgrpc.NewChatServer(chatService))

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		fmt.Printf("chat-service: listening on :%s\n", port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("chat-service: serve failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("chat-service: shutting down")
	grpcServer.GracefulStop()
}
