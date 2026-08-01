package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

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
	mux.Handle("/ws", ws.NewHandler(jwtSecret))

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
