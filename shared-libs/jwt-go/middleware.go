package jwt

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

func AuthMiddleware(jwtService *Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Authorization header missing",
			})
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid authorization header format",
			})
		}

		tokenString := parts[1]

		claims, err := jwtService.ValidateAccessToken(tokenString)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}

		c.Locals("userID", claims.UserID)
		c.Locals("userEmail", claims.Email)
		c.Locals("subscriptionPlan", claims.SubscriptionPlan)
		c.Locals("claims", claims)

		return c.Next()
	}
}

func OptionalAuth(jwtService *Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Next()
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Next()
		}

		tokenString := parts[1]
		claims, err := jwtService.ValidateAccessToken(tokenString)
		if err != nil {
			return c.Next()
		}

		c.Locals("userID", claims.UserID)
		c.Locals("userEmail", claims.Email)
		c.Locals("subscriptionPlan", claims.SubscriptionPlan)
		c.Locals("claims", claims)

		return c.Next()
	}
}

func GetUserID(c fiber.Ctx) (uuid.UUID, bool) {
	userID, ok := c.Locals("userID").(uuid.UUID)
	return userID, ok
}

func GetUserIDString(c fiber.Ctx) (string, bool) {
	userID, ok := GetUserID(c)
	if !ok {
		return "", false
	}
	return userID.String(), true
}

func GetUserEmail(c fiber.Ctx) (string, bool) {
	email, ok := c.Locals("userEmail").(string)
	return email, ok
}

func GetSubscriptionPlan(c fiber.Ctx) (string, bool) {
	plan, ok := c.Locals("subscriptionPlan").(string)
	return plan, ok
}

func GetClaims(c fiber.Ctx) (*Claims, bool) {
	claims, ok := c.Locals("claims").(*Claims)
	return claims, ok
}

func RequireSubscription(requiredPlans ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		plan, ok := GetSubscriptionPlan(c)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "User not authenticated",
			})
		}

		for _, requiredPlan := range requiredPlans {
			if plan == requiredPlan {
				return c.Next()
			}
		}

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Subscription plan not sufficient",
			"details": fiber.Map{
				"current_plan":  plan,
				"required_plan": requiredPlans,
			},
		})
	}
}

func GetLocals[T any](c fiber.Ctx, key string) (T, bool) {
	value := c.Locals(key)
	if value == nil {
		var zero T
		return zero, false
	}

	typed, ok := value.(T)
	return typed, ok
}

func ExtractTokenFromHeader(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errors.New("authorization header is missing")
	}

	const bearerPrefix = "Bearer "
	if len(authHeader) < len(bearerPrefix) || authHeader[:len(bearerPrefix)] != bearerPrefix {
		return "", errors.New("invalid authorization header format")
	}

	return authHeader[len(bearerPrefix):], nil
}
