SERVICES := auth-service chat-service ws-gateway notification-worker

.PHONY: proto up down build-images logs unit tidy ci integration

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

integration:
	cd services/auth-service && go test -tags=integration ./... -v

tidy:
	cd proto/gen && go mod tidy
	@for prime in $(SERVICES); do \
		echo "== go mod tidy: $$prime =="; \
		(cd services/$$prime && go mod tidy) || exit 1; \
	done

ci:
	@echo "== protolint =="
	protolint lint proto/auth/v1/auth.proto proto/chat/v1/chat.proto
	@for s in $(SERVICES); do \
		echo "== $$s: gofmt =="; \
		fmt_out=$$(cd services/$$s && gofmt -l .); \
		if [ -n "$$fmt_out" ]; then \
			echo "not gofmt'ed:"; echo "$$fmt_out"; exit 1; \
		fi; \
		echo "== $$s: go mod tidy check =="; \
		(cd services/$$s && go mod tidy && git diff --exit-code -- go.mod go.sum) || exit 1; \
		echo "== $$s: go vet =="; \
		(cd services/$$s && go vet ./...) || exit 1; \
		echo "== $$s: golangci-lint =="; \
		(cd services/$$s && golangci-lint run ./...) || exit 1; \
		echo "== $$s: go build =="; \
		(cd services/$$s && go build ./...) || exit 1; \
		echo "== $$s: go test =="; \
		(cd services/$$s && go test ./... -cover -coverprofile=cover.out) || exit 1; \
	done
