package zoho

import (
	"context"
	"time"
)

// withRetry calls fn up to maxAttempts times with exponential backoff (1s, 2s, 4s).
// Stops early on non-retryable errors or context cancellation.
func withRetry(ctx context.Context, maxAttempts int, fn func() error) error {
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if lastErr = fn(); lastErr == nil {
			return nil
		}
		if !isRetryable(lastErr) {
			return lastErr
		}
		delay := time.Duration(1<<uint(i)) * time.Second
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}
