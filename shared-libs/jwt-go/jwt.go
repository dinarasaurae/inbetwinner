package jwt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Service struct {
	secretKey            []byte
	accessTokenDuration  time.Duration
	refreshTokenDuration time.Duration
	issuer               string
}

func NewService(secretKey string, accessTokenDuration, refreshTokenDuration time.Duration) *Service {
	return &Service{
		secretKey:            []byte(secretKey),
		accessTokenDuration:  accessTokenDuration,
		refreshTokenDuration: refreshTokenDuration,
		issuer:               "inbetwin-auth-service",
	}
}

func (s *Service) GenerateToken(userID string) (string, error) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return "", err
	}
	return s.GenerateAccessToken(userUUID, "", "")
}

func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	return s.ValidateAccessToken(tokenString)
}

func (s *Service) GenerateAccessToken(userID uuid.UUID, email, subscriptionPlan string) (string, error) {
	now := time.Now()
	expirationTime := now.Add(s.accessTokenDuration)

	claims := &Claims{
		UserID:           userID,
		Email:            email,
		SubscriptionPlan: subscriptionPlan,
		TokenType:        "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    s.issuer,
			Subject:   userID.String(),
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.secretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (s *Service) GenerateRefreshToken(userID uuid.UUID) (string, error) {
	now := time.Now()
	expirationTime := now.Add(s.refreshTokenDuration)

	claims := &Claims{
		UserID:    userID,
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    s.issuer,
			Subject:   userID.String(),
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.secretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (s *Service) ValidateAccessToken(tokenString string) (*Claims, error) {
	return s.validateToken(tokenString, "access")
}

func (s *Service) ValidateRefreshToken(tokenString string) (*Claims, error) {
	return s.validateToken(tokenString, "refresh")
}

func (s *Service) validateToken(tokenString, expectedType string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return s.secretKey, nil
		},
	)

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	if expectedType != "" && claims.TokenType != expectedType {
		return nil, errors.New("invalid token type")
	}

	if claims.Issuer != s.issuer {
		return nil, errors.New("invalid token issuer")
	}

	return claims, nil
}

func (s *Service) HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *Service) GetAccessTokenExpiration() int {
	return int(s.accessTokenDuration.Seconds())
}

func (s *Service) GetRefreshTokenExpiration() time.Time {
	return time.Now().Add(s.refreshTokenDuration)
}
