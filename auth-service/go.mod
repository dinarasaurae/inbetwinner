module github.com/dinarasaurae/inbetwin-auth-service

go 1.26

require (
	github.com/dinarasaurae/inbetwin-shared/jwt-go v0.0.0
	github.com/gofiber/fiber/v3 v3.0.0-beta.3
	github.com/golang-jwt/jwt/v5 v5.2.0
	github.com/golang-migrate/migrate/v4 v4.17.0
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.10.9
	golang.org/x/crypto v0.24.0
)

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/gofiber/utils/v2 v2.0.0-beta.4 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.55.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
)

replace github.com/dinarasaurae/inbetwin-shared/jwt-go => ../shared-libs/jwt-go
