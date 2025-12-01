package services

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/dinarasaurae/inbetwin-auth-service/internal/database"
	"github.com/dinarasaurae/inbetwin-auth-service/internal/models"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
)

type AuthService struct {
	db         *database.DB
	jwtService *jwtlib.Service
}

func NewAuthService(db *database.DB, jwtService *jwtlib.Service) *AuthService {
	return &AuthService{
		db:         db,
		jwtService: jwtService,
	}
}

func (s *AuthService) Register(req *models.CreateUserRequest) (*models.User, error) {
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", req.Email).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("user with this email already exists")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:               uuid.New(),
		Email:            req.Email,
		PasswordHash:     stringPtr(string(passwordHash)),
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		EmailVerified:    false,
		SubscriptionPlan: "free",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	query := `
		INSERT INTO users (id, email, password_hash, first_name, last_name, email_verified, subscription_plan, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = s.db.Exec(query,
		user.ID, user.Email, user.PasswordHash, user.FirstName, user.LastName,
		user.EmailVerified, user.SubscriptionPlan, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return nil, err
	}

	subscription := &models.Subscription{
		ID:          uuid.New(),
		UserID:      user.ID,
		Plan:        "free",
		Status:      "trialing", // 7 дней trial
		StartedAt:   time.Now(),
		TrialEndsAt: timePtr(time.Now().Add(7 * 24 * time.Hour)),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	subscriptionQuery := `
		INSERT INTO subscriptions (id, user_id, plan, status, started_at, trial_ends_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err = s.db.Exec(subscriptionQuery,
		subscription.ID, subscription.UserID, subscription.Plan, subscription.Status,
		subscription.StartedAt, subscription.TrialEndsAt, subscription.CreatedAt, subscription.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) Login(req *models.LoginRequest) (*models.User, string, string, error) {
	user := &models.User{}
	query := `
		SELECT id, email, password_hash, first_name, last_name, avatar_url,
		       email_verified, subscription_plan, created_at, updated_at
		FROM users WHERE email = $1
	`
	err := s.db.QueryRow(query, req.Email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.FirstName, &user.LastName,
		&user.AvatarURL, &user.EmailVerified, &user.SubscriptionPlan,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", "", errors.New("invalid email or password")
		}
		return nil, "", "", err
	}

	if user.PasswordHash == nil {
		return nil, "", "", errors.New("this account was created via OAuth, please use social login")
	}

	err = bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password))
	if err != nil {
		return nil, "", "", errors.New("invalid email or password")
	}

	accessToken, err := s.jwtService.GenerateAccessToken(user.ID, user.Email, user.SubscriptionPlan)
	if err != nil {
		return nil, "", "", err
	}

	refreshToken, err := s.jwtService.GenerateRefreshToken(user.ID)
	if err != nil {
		return nil, "", "", err
	}

	err = s.saveRefreshToken(user.ID, refreshToken, "web")
	if err != nil {
		return nil, "", "", err
	}

	return user, accessToken, refreshToken, nil
}

func (s *AuthService) RefreshAccessToken(refreshToken string) (string, error) {
	claims, err := s.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		return "", errors.New("invalid refresh token")
	}

	tokenHash := s.jwtService.HashToken(refreshToken)
	var exists bool
	err = s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE token_hash = $1 AND user_id = $2)",
		tokenHash, claims.UserID).Scan(&exists)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", errors.New("refresh token not found")
	}

	var email, subscriptionPlan string
	err = s.db.QueryRow("SELECT email, subscription_plan FROM users WHERE id = $1",
		claims.UserID).Scan(&email, &subscriptionPlan)
	if err != nil {
		return "", err
	}

	accessToken, err := s.jwtService.GenerateAccessToken(claims.UserID, email, subscriptionPlan)
	if err != nil {
		return "", err
	}

	return accessToken, nil
}

func (s *AuthService) GetUserByID(userID uuid.UUID) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, email, password_hash, first_name, last_name, avatar_url,
		       email_verified, subscription_plan, created_at, updated_at
		FROM users WHERE id = $1
	`
	err := s.db.QueryRow(query, userID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.FirstName, &user.LastName,
		&user.AvatarURL, &user.EmailVerified, &user.SubscriptionPlan,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	return user, nil
}

func (s *AuthService) saveRefreshToken(userID uuid.UUID, refreshToken, deviceInfo string) error {
	tokenHash := s.jwtService.HashToken(refreshToken)
	expiresAt := s.jwtService.GetRefreshTokenExpiration()

	query := `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, device_info, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := s.db.Exec(query, uuid.New(), userID, tokenHash, expiresAt, deviceInfo, time.Now())
	return err
}

func stringPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
