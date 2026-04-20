package amocrm

import (
	"errors"
	"fmt"
)

// APIError is returned when AmoCRM responds with an HTTP error status.
// Keeping StatusCode as a field lets withRetry decide whether to retry.
type APIError struct {
	StatusCode int
	Body       []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("amocrm API %d: %s", e.StatusCode, e.Body)
}

// isRetryable returns true for server-side (5xx) and network errors.
// 4xx errors (bad request, unauthorized) are not retried.
func isRetryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500
	}
	// Any non-API error is assumed to be a network/transport error → retry.
	return true
}
