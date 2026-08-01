module github.com/VladimirKhmelev/messenger-on-go/services/ws-gateway

go 1.26.2

require (
	github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil v0.0.0-00010101000000-000000000000
	github.com/VladimirKhmelev/messenger-on-go/proto/gen v0.0.0-00010101000000-000000000000
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/gorilla/websocket v1.5.3
	google.golang.org/grpc v1.82.1
)

replace (
	github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil => ../../pkg/jwtutil
	github.com/VladimirKhmelev/messenger-on-go/proto/gen => ../../proto/gen
)

require (
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
