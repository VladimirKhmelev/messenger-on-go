package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	authv1 "github.com/VladimirKhmelev/messenger-on-go/proto/gen/auth/v1"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/cache"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/jwtutil"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/mail"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/repository"
	"github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/service"
	transportgrpc "github.com/VladimirKhmelev/messenger-on-go/services/auth-service/internal/transport/grpc"
)

func main() {
	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("auth-service: POSTGRES_DSN is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("auth-service: JWT_SECRET is required")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("auth-service: REDIS_ADDR is required")
	}

	smtpAddr := os.Getenv("SMTP_ADDR")
	if smtpAddr == "" {
		log.Fatal("auth-service: SMTP_ADDR is required")
	}

	smtpFrom := os.Getenv("SMTP_FROM")
	if smtpFrom == "" {
		log.Fatal("auth-service: SMTP_FROM is required")
	}

	userRepo, err := repository.NewPostgresUserRepository(dsn)
	if err != nil {
		log.Fatalf("auth-service: failed to connect to postgres: %v", err)
	}

	if err := userRepo.Migrate(); err != nil {
		log.Fatalf("auth-service: failed to run migrations: %v", err)
	}

	redisClient := cache.NewClient(redisAddr)
	loginLimiter := cache.NewLoginRateLimiter(redisClient)
	refreshBlocklist := cache.NewTokenBlacklist(redisClient)
	emailCodes := cache.NewEmailVerificationStore(redisClient)
	mailer := mail.NewSender(smtpAddr, smtpFrom)

	tokenIssuer := jwtutil.NewIssuer(jwtSecret)
	authService := service.NewAuthService(userRepo, tokenIssuer, loginLimiter, refreshBlocklist, emailCodes, mailer)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("auth-service: failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(transportgrpc.AuthInterceptor(tokenIssuer)),
	)

	authv1.RegisterAuthServiceServer(grpcServer, transportgrpc.NewAuthServer(authService))

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		fmt.Printf("auth-service: listening on :%s\n", port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("auth-service: serve failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	fmt.Println("auth-service: shutting down")
	grpcServer.GracefulStop()
}
