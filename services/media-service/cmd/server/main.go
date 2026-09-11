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
	mediav1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/media/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/chatclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/minioclient"
	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/repository"
	"github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/service"
	transportgrpc "github.com/VladimirKhmelev/messenger-on-go/services/media-service/internal/transport/grpc"
)

const mediaBucket = "messenger-media"

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
		log.Fatal("media-service: POSTGRES_DSN is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("media-service: JWT_SECRET is required")
	}

	internalSecret := os.Getenv("INTERNAL_SECRET")
	if internalSecret == "" {
		log.Fatal("media-service: INTERNAL_SECRET is required")
	}

	chatServiceAddr := os.Getenv("CHAT_SERVICE_ADDR")
	if chatServiceAddr == "" {
		log.Fatal("media-service: CHAT_SERVICE_ADDR is required")
	}

	minioEndpoint := os.Getenv("MINIO_ENDPOINT")
	if minioEndpoint == "" {
		log.Fatal("media-service: MINIO_ENDPOINT is required")
	}

	minioAccessKey := os.Getenv("MINIO_ROOT_USER")
	if minioAccessKey == "" {
		log.Fatal("media-service: MINIO_ROOT_USER is required")
	}

	minioSecretKey := os.Getenv("MINIO_ROOT_PASSWORD")
	if minioSecretKey == "" {
		log.Fatal("media-service: MINIO_ROOT_PASSWORD is required")
	}

	publicMediaURL := os.Getenv("PUBLIC_MEDIA_URL")
	if publicMediaURL == "" {
		log.Fatal("media-service: PUBLIC_MEDIA_URL is required")
	}

	tracingShutdown, err := tracing.Setup(context.Background(), "media-service", os.Getenv("JAEGER_ENDPOINT"))
	if err != nil {
		log.Fatalf("media-service: failed to set up tracing: %v", err)
	}
	defer func() {
		if err := tracingShutdown(context.Background()); err != nil {
			log.Printf("media-service: tracing shutdown failed: %v", err)
		}
	}()

	mediaRepo, err := repository.NewPostgresMediaRepository(dsn)
	if err != nil {
		log.Fatalf("media-service: failed to connect to postgres: %v", err)
	}

	if err := mediaRepo.Migrate(); err != nil {
		log.Fatalf("media-service: failed to run migrations: %v", err)
	}

	store, err := minioclient.New(minioEndpoint, minioAccessKey, minioSecretKey, mediaBucket, publicMediaURL)
	if err != nil {
		log.Fatalf("media-service: failed to create MinIO client: %v", err)
	}
	if err := store.EnsureBucket(context.Background()); err != nil {
		log.Fatalf("media-service: failed to ensure bucket: %v", err)
	}

	chatClient, err := chatclient.Dial(chatServiceAddr, internalSecret)
	if err != nil {
		log.Fatalf("media-service: failed to dial chat-service: %v", err)
	}
	defer func() { _ = chatClient.Close() }()

	mediaService := service.NewMediaService(mediaRepo, store, chatClient)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("media-service: failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			metrics.UnaryServerInterceptor("media-service"),
			transportgrpc.AuthInterceptor(jwtSecret),
		),
	)

	mediav1.RegisterMediaServiceServer(grpcServer, transportgrpc.NewMediaServer(mediaService))

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		fmt.Printf("media-service: grpc listening on :%s\n", port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("media-service: serve failed: %v", err)
		}
	}()

	gwMux := runtime.NewServeMux()
	gwOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(metrics.UnaryClientInterceptor("media-service")),
	}
	if err := mediav1.RegisterMediaServiceHandlerFromEndpoint(context.Background(), gwMux, "localhost:"+port, gwOpts); err != nil {
		log.Fatalf("media-service: failed to register HTTP gateway: %v", err)
	}
	if err := gwMux.HandlePath(http.MethodGet, "/metrics", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		metrics.Handler(w, r)
	}); err != nil {
		log.Fatalf("media-service: failed to register /metrics route: %v", err)
	}

	httpServer := &http.Server{
		Addr:    ":" + httpPort,
		Handler: gwMux,
	}

	go func() {
		fmt.Printf("media-service: http listening on :%s\n", httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("media-service: http serve failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("media-service: shutting down")
	grpcServer.GracefulStop()
	_ = httpServer.Close()
}
