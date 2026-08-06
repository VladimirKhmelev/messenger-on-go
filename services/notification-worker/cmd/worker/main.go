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

	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/chatclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/notification-worker/internal/notify"
)

func main() {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		log.Fatal("notification-worker: NATS_URL is required")
	}

	chatServiceAddr := os.Getenv("CHAT_SERVICE_ADDR")
	if chatServiceAddr == "" {
		log.Fatal("notification-worker: CHAT_SERVICE_ADDR is required")
	}

	internalSecret := os.Getenv("INTERNAL_SECRET")
	if internalSecret == "" {
		log.Fatal("notification-worker: INTERNAL_SECRET is required")
	}

	chatClient, err := chatclient.Dial(chatServiceAddr, internalSecret)
	if err != nil {
		log.Fatalf("notification-worker: failed to dial chat-service: %v", err)
	}
	defer func() { _ = chatClient.Close() }()

	handler := notify.NewHandler(chatClient)

	subs := []events.Subscription{
		{
			StreamName:    "CHAT_EVENTS",
			ConsumerName:  "notification-worker-chat-events",
			FilterSubject: "msg.created",
			Handle:        handler.HandleMessageCreated,
		},
		{
			StreamName:    "USER_EVENTS",
			ConsumerName:  "notification-worker-user-events",
			FilterSubject: "user.*",
			Handle:        handler.HandleUserEvent,
		},
	}

	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	defer cancelConsumer()

	go func() {
		if err := events.Consume(consumerCtx, natsURL, subs); err != nil {
			log.Fatalf("notification-worker: NATS consumer failed: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("notification-worker: failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		fmt.Printf("notification-worker: listening on :%s\n", port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("notification-worker: serve failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("notification-worker: shutting down")
	grpcServer.GracefulStop()
}
