package zoho

import (
	"errors"
	"fmt"
)

// APIError is returned when Zoho CRM responds with an HTTP error status.
type APIError struct {
	StatusCode int
	Body       []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("zoho API %d: %s", e.StatusCode, e.Body)
}

// isRetryable returns true for 5xx and network errors; 4xx errors are not retried.
func isRetryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500
	}
	return true // network/transport error → retry
}
