module github.com/VladimirKhmelev/messenger-on-go/services/notification-worker

go 1.26.2

require (
	github.com/VladimirKhmelev/messenger-on-go/proto/gen v0.0.0-00010101000000-000000000000
	github.com/nats-io/nats.go v1.52.0
	google.golang.org/grpc v1.83.0
)

replace github.com/VladimirKhmelev/messenger-on-go/proto/gen => ../../proto/gen

require (
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260803160001-6ac0973c030d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260803160001-6ac0973c030d // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
