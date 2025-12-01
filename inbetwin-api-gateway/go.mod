module github.com/dinarasaurae/inbetwin-api-gateway

go 1.25.0

require (
	github.com/dinarasaurae/inbetwin-shared/jwt-go v0.0.0
	github.com/gofiber/fiber/v3 v3.0.0-rc.2
	github.com/joho/godotenv v1.5.1
	github.com/redis/go-redis/v9 v9.16.0
	github.com/valyala/fasthttp v1.65.0
)

require (
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gofiber/schema v1.6.0 // indirect
	github.com/gofiber/utils/v2 v2.0.0-rc.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/tinylib/msgp v1.4.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	golang.org/x/crypto v0.42.0 // indirect
	golang.org/x/net v0.44.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

// Используем локальную версию shared библиотеки
replace github.com/dinarasaurae/inbetwin-shared/jwt-go => ../shared-libs/jwt-go
