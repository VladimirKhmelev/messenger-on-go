.PHONY: proto

proto:
	protoc --proto_path=proto \
		--go_out=proto/gen --go_opt=module=github.com/VladimirKhmelev/messenger-on-go/proto/gen \
		--go-grpc_out=proto/gen --go-grpc_opt=module=github.com/VladimirKhmelev/messenger-on-go/proto/gen \
		proto/auth/v1/auth.proto proto/chat/v1/chat.proto
