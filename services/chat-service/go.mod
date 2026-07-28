module github.com/VladimirKhmelev/messenger-on-go/services/chat-service

go 1.26.2

require (
	github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil v0.0.0-00010101000000-000000000000
	github.com/VladimirKhmelev/messenger-on-go/proto/gen v0.0.0-00010101000000-000000000000
	github.com/golang-migrate/migrate/v4 v4.18.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.9.2
	github.com/jmoiron/sqlx v1.4.0
	google.golang.org/grpc v1.82.1
)

replace (
	github.com/VladimirKhmelev/messenger-on-go/pkg/jwtutil => ../../pkg/jwtutil
	github.com/VladimirKhmelev/messenger-on-go/proto/gen => ../../proto/gen
)

require (
	github.com/docker/go-connections v0.6.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.14.3 // indirect
	github.com/jackc/pgerrcode v0.0.0-20220416144525-469b46aa5efa // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.3 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgtype v1.14.0 // indirect
	github.com/jackc/pgx/v4 v4.18.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
