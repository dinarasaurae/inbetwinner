package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	registry = prometheus.NewRegistry()

	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "inbetwin_api_gateway_http_requests_total",
			Help: "Total number of HTTP requests handled by the API gateway.",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "inbetwin_api_gateway_http_request_duration_seconds",
			Help:    "HTTP request duration at the API gateway in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "route", "status"},
	)

	httpInflightRequests = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "inbetwin_api_gateway_http_inflight_requests",
			Help: "Current number of in-flight HTTP requests at the API gateway.",
		},
	)
)

func init() {
	registry.MustRegister(
		httpRequestsTotal,
		httpRequestDuration,
		httpInflightRequests,
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
}

func Registry() *prometheus.Registry {
	return registry
}

func HTTPMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		httpInflightRequests.Inc()

		err := c.Next()

		httpInflightRequests.Dec()

		status := c.Response().StatusCode()
		if err != nil && status < fiber.StatusBadRequest {
			status = fiber.StatusInternalServerError
		}

		labels := prometheus.Labels{
			"method": c.Method(),
			"route":  routeLabel(c),
			"status": strconv.Itoa(status),
		}

		httpRequestsTotal.With(labels).Inc()
		httpRequestDuration.With(labels).Observe(time.Since(start).Seconds())

		return err
	}
}

func routeLabel(c fiber.Ctx) string {
	route := c.Route()
	if route != nil && route.Path != "" {
		return route.Path
	}

	if c.Matched() {
		return c.Path()
	}

	return "unmatched"
}
