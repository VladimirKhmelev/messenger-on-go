SERVICES := auth-service chat-service ws-gateway notification-worker

.PHONY: proto up down build-images logs unit tidy ci integration lint protolint golangci-lint

proto:
	mkdir -p proto/gen/openapi
	protoc --proto_path=proto --proto_path=proto/third_party \
		--go_out=proto/gen --go_opt=module=github.com/VladimirKhmelev/messenger-on-go/proto/gen \
		--go-grpc_out=proto/gen --go-grpc_opt=module=github.com/VladimirKhmelev/messenger-on-go/proto/gen \
		--grpc-gateway_out=proto/gen --grpc-gateway_opt=module=github.com/VladimirKhmelev/messenger-on-go/proto/gen \
		--grpc-gateway_opt=generate_unbound_methods=true \
		--openapiv2_out=proto/gen/openapi --openapiv2_opt=allow_merge=true,merge_file_name=messenger \
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
	@echo "mode: set" > cover-all.out
	@for s in $(SERVICES); do \
		echo "== $$s: go test =="; \
		(cd services/$$s && go test ./... -cover -coverprofile=cover.out) || exit 1; \
		(cd services/$$s && go tool cover -func=cover.out); \
		tail -n +2 services/$$s/cover.out >> cover-all.out; \
	done
	go tool cover -html=cover-all.out -o cover-all.html
	xdg-open cover-all.html 2>/dev/null || open cover-all.html 2>/dev/null || true

integration:
	cd services/auth-service && go test -tags=integration ./... -v

lint: protolint golangci-lint

protolint:
	protolint lint proto/auth/v1/auth.proto proto/chat/v1/chat.proto

golangci-lint:
	@for s in $(SERVICES); do \
		echo "== $$s: golangci-lint =="; \
		(cd services/$$s && golangci-lint run ./...) || exit 1; \
	done

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
