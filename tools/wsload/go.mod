module github.com/VladimirKhmelev/messenger-on-go/tools/wsload

go 1.26.3

require (
	github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil v0.0.0-00010101000000-000000000000
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/gorilla/websocket v1.5.3
)

replace github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil => ../../pkg/jwtutil
