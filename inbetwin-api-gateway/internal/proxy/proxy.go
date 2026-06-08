package proxy

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

type Service struct {
	client   *fasthttp.Client
	reqPool  *sync.Pool
	respPool *sync.Pool
}

func NewService() *Service {
	return &Service{
		client: &fasthttp.Client{
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,

			NoDefaultUserAgentHeader:      true,
			DisableHeaderNamesNormalizing: true,
			DisablePathNormalizing:        true,

			MaxConnsPerHost:     512,
			MaxIdleConnDuration: 30 * time.Second,
			MaxConnDuration:     0,

			MaxIdemponentCallAttempts: 2,
			RetryIfErr: func(request *fasthttp.Request, attempts int, err error) (bool, bool) {
				if !isRetryableMethod(string(request.Header.Method())) {
					return false, false
				}

				log.Printf("proxy retry after downstream error: method=%s target=%s attempt=%d err=%v",
					request.Header.Method(), request.URI().Path(), attempts, err)

				return true, true
			},
		},

		// Object pools для минимизации GC pressure
		reqPool: &sync.Pool{
			New: func() interface{} {
				return &fasthttp.Request{}
			},
		},
		respPool: &sync.Pool{
			New: func() interface{} {
				return &fasthttp.Response{}
			},
		},
	}
}

var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailers":            true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

func (s *Service) ProxyRequest(c fiber.Ctx, targetURL string) error {
	// Получаем объекты из пула для минимизации аллокаций
	req := s.reqPool.Get().(*fasthttp.Request)
	resp := s.respPool.Get().(*fasthttp.Response)
	defer func() {
		// Очищаем и возвращаем в пул
		req.Reset()
		resp.Reset()
		s.reqPool.Put(req)
		s.respPool.Put(resp)
	}()

	originalPath := c.Path()

	// Для разных сервисов разная логика проксирования
	var targetPath string
	if strings.Contains(targetURL, "auth-service") {
		// Auth Service ожидает полный путь /api/v1/auth/...
		targetPath = originalPath
	} else {
		// Другие сервисы могут ожидать путь без /api/v1
		targetPath = strings.TrimPrefix(originalPath, "/api/v1")
	}

	req.SetRequestURI(targetURL + targetPath)

	if queryString := c.Request().URI().QueryString(); len(queryString) > 0 {
		req.URI().SetQueryStringBytes(queryString)
	}

	req.Header.SetMethod(c.Method())

	req.SetBody(c.Body())

	for key, value := range c.Request().Header.All() {
		if !hopByHopHeaders[string(key)] {
			req.Header.SetBytesKV(key, value)
		}
	}

	req.Header.Set("X-Forwarded-For", c.IP())
	req.Header.Set("X-Forwarded-Proto", c.Protocol())
	req.Header.Set("X-Forwarded-Host", c.Hostname())
	req.Header.Set("X-Real-IP", c.IP())

	if userID := c.Locals("userID"); userID != nil {
		req.Header.Set("X-User-ID", userID.(uuid.UUID).String())
	}

	err := s.client.Do(req, resp)
	if err != nil {
		log.Printf("proxy downstream error: method=%s path=%s target=%s err=%v",
			c.Method(), c.Path(), targetURL+targetPath, err)

		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "service unavailable",
			"service": targetURL,
			"details": err.Error(),
		})
	}

	c.Status(resp.StatusCode())

	for key, value := range resp.Header.All() {
		if !hopByHopHeaders[string(key)] {
			c.Set(string(key), string(value))
		}
	}

	return c.Send(resp.Body())
}

func isRetryableMethod(method string) bool {
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}
