package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/VladimirKhmelev/messenger-on-go/pkg/metrics"
	"github.com/VladimirKhmelev/messenger-on-go/pkg/tracing"
	chatv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/chat/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/authclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/cache"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/events"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/repository"
	"github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/service"
	transportgrpc "github.com/VladimirKhmelev/messenger-on-go/services/chat-service/internal/transport/grpc"
)

const maxGRPCRecvMsgSize = 256 * 1024

func main() {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}

	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
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

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("chat-service: REDIS_ADDR is required")
	}

	internalSecret := os.Getenv("INTERNAL_SECRET")
	if internalSecret == "" {
		log.Fatal("chat-service: INTERNAL_SECRET is required")
	}

	tracingShutdown, err := tracing.Setup(context.Background(), "chat-service", os.Getenv("JAEGER_ENDPOINT"))
	if err != nil {
		log.Fatalf("chat-service: failed to set up tracing: %v", err)
	}
	defer func() {
		if err := tracingShutdown(context.Background()); err != nil {
			log.Printf("chat-service: tracing shutdown failed: %v", err)
		}
	}()

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

	redisClient := cache.NewClient(redisAddr)
	presenceStore := cache.NewPresenceStore(redisClient)
	sendLimiter := cache.NewSendMessageRateLimiter(redisClient)

	chatService := service.NewChatService(chatRepo, authClient, eventPublisher, presenceStore, sendLimiter)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("chat-service: failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			metrics.UnaryServerInterceptor("chat-service"),
			transportgrpc.AuthInterceptor(jwtSecret, internalSecret),
		),
		grpc.MaxRecvMsgSize(maxGRPCRecvMsgSize),
	)

	chatv1.RegisterChatServiceServer(grpcServer, transportgrpc.NewChatServer(chatService))

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		fmt.Printf("chat-service: grpc listening on :%s\n", port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("chat-service: serve failed: %v", err)
		}
	}()

	gwMux := runtime.NewServeMux()
	gwOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(metrics.UnaryClientInterceptor("chat-service")),
	}
	if err := chatv1.RegisterChatServiceHandlerFromEndpoint(context.Background(), gwMux, "localhost:"+port, gwOpts); err != nil {
		log.Fatalf("chat-service: failed to register HTTP gateway: %v", err)
	}
	if err := gwMux.HandlePath(http.MethodGet, "/metrics", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		metrics.Handler(w, r)
	}); err != nil {
		log.Fatalf("chat-service: failed to register /metrics route: %v", err)
	}

	httpServer := &http.Server{
		Addr:    ":" + httpPort,
		Handler: gwMux,
	}

	go func() {
		fmt.Printf("chat-service: http listening on :%s\n", httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("chat-service: http serve failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("chat-service: shutting down")
	grpcServer.GracefulStop()
	_ = httpServer.Close()
}
