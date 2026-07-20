.PHONY: proto up down build-images logs unit

proto:
	protoc --proto_path=proto \
		--go_out=proto/gen --go_opt=module=github.com/VladimirKhmelev/messenger-on-go/proto/gen \
		--go-grpc_out=proto/gen --go-grpc_opt=module=github.com/VladimirKhmelev/messenger-on-go/proto/gen \
		proto/auth/v1/auth.proto proto/chat/v1/chat.proto

up:
	docker-compose up -d --build

down:
	docker-compose down

build-images:
	docker-compose build

logs:
	docker-compose logs -f

unit:
	cd services/auth-service && go test ./... -cover -coverprofile=cover.out
	cd services/auth-service && go tool cover -func=cover.out
	cd services/auth-service && go tool cover -html=cover.out -o cover.html
	xdg-open services/auth-service/cover.html 2>/dev/null || open services/auth-service/cover.html 2>/dev/null || true
