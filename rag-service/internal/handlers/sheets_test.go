package handlers

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
)

func TestStatusForSheetsSyncError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "bad request error maps to 400",
			err: &services.RequestError{
				Status:  http.StatusBadRequest,
				Message: "invalid spreadsheet_id",
			},
			want: fiber.StatusBadRequest,
		},
		{
			name: "wrapped request error keeps original status",
			err: fmt.Errorf("get sheet: %w", &services.RequestError{
				Status:  http.StatusUnprocessableEntity,
				Message: "google sheets returned 400",
			}),
			want: fiber.StatusUnprocessableEntity,
		},
		{
			name: "unknown errors stay 500",
			err:  fmt.Errorf("db is down"),
			want: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := statusForSheetsSyncError(tt.err); got != tt.want {
				t.Fatalf("statusForSheetsSyncError() = %d, want %d", got, tt.want)
			}
		})
	}
}
