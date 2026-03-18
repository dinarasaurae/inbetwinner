package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
)

// RequireConsent checks that an active consent row exists for the given platform
// before allowing the handler to run. Must be used after jwtlib.AuthMiddleware.
func RequireConsent(db *database.DB, platform string) fiber.Handler {
	return func(c fiber.Ctx) error {
		userID, ok := jwtlib.GetUserID(c)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"status": "error",
				"error":  "unauthenticated",
			})
		}

		var exists bool
		err := db.QueryRowContext(
			c.Context(),
			`SELECT EXISTS(
				SELECT 1 FROM user_consents
				WHERE user_id = $1
				  AND platform = $2
				  AND revoked_at IS NULL
			)`,
			userID,
			platform,
		).Scan(&exists)

		if err != nil || !exists {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"status":  "error",
				"error":   "consent_required",
				"message": "You must grant consent for " + platform + " integration before accessing this resource.",
			})
		}

		return c.Next()
	}
}

// userIDFromLocals is a helper used in tests.
func userIDFromLocals(c fiber.Ctx) (uuid.UUID, bool) {
	return jwtlib.GetUserID(c)
}
