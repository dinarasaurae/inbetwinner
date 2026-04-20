package amocrm

import (
	"context"
	"time"
)

// withRetry calls fn up to maxAttempts times, backing off exponentially (1s, 2s, 4s, …).
// It stops early if the error is not retryable (see isRetryable) or the context is cancelled.
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
		delay := time.Duration(1<<uint(i)) * time.Second // 1s, 2s, 4s
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}
