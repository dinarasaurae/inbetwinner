package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	"github.com/dinarasaurae/inbetwin-auth-service/internal/database"
	"github.com/dinarasaurae/inbetwin-auth-service/internal/models"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
)

type AuthService struct {
	db         *database.DB
	jwtService *jwtlib.Service
}

var ErrUserAlreadyExists = errors.New("user with this email already exists")

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
		return nil, ErrUserAlreadyExists
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:               uuid.New(),
		Email:            req.Email,
		PasswordHash:     stringPtr(string(passwordHash)),
		Name:             req.Name, // добавлено поле name
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		EmailVerified:    false,
		SubscriptionPlan: "free",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	query := `
		INSERT INTO users (id, email, password_hash, name, first_name, last_name, email_verified, subscription_plan, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err = s.db.Exec(query,
		user.ID, user.Email, user.PasswordHash, user.Name, user.FirstName, user.LastName,
		user.EmailVerified, user.SubscriptionPlan, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, ErrUserAlreadyExists
		}
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
		SELECT id, email, password_hash, name, first_name, last_name, avatar_url,
		       email_verified, subscription_plan, created_at, updated_at
		FROM users WHERE email = $1
	`
	err := s.db.QueryRow(query, req.Email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.FirstName, &user.LastName,
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
		SELECT id, email, password_hash, name, first_name, last_name, avatar_url,
		       email_verified, subscription_plan, created_at, updated_at
		FROM users WHERE id = $1
	`
	err := s.db.QueryRow(query, userID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.FirstName, &user.LastName,
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

func (s *AuthService) UpdateUserProfile(userID uuid.UUID, req *models.UpdateUserRequest) (*models.User, error) {
	if req.FirstName == nil && req.LastName == nil && req.AvatarURL == nil {
		return nil, errors.New("at least one field is required")
	}

	// Keep `name` in sync for clients that still rely on a single full name field.
	var computedName *string
	first := ""
	last := ""
	if req.FirstName != nil {
		first = strings.TrimSpace(*req.FirstName)
	}
	if req.LastName != nil {
		last = strings.TrimSpace(*req.LastName)
	}
	full := strings.TrimSpace(strings.Join([]string{first, last}, " "))
	if full != "" {
		computedName = &full
	}

	query := `
		UPDATE users
		SET
			first_name = COALESCE($2, first_name),
			last_name = COALESCE($3, last_name),
			avatar_url = COALESCE($4, avatar_url),
			name = COALESCE($5, name),
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, email, password_hash, name, first_name, last_name, avatar_url,
		          email_verified, subscription_plan, created_at, updated_at
	`

	user := &models.User{}
	err := s.db.QueryRow(query, userID, req.FirstName, req.LastName, req.AvatarURL, computedName).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.FirstName, &user.LastName,
		&user.AvatarURL, &user.EmailVerified, &user.SubscriptionPlan, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	return user, nil
}

func (s *AuthService) ChangePassword(userID uuid.UUID, req *models.ChangePasswordRequest) error {
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return errors.New("current_password and new_password are required")
	}
	if len(req.NewPassword) < 6 {
		return errors.New("new password must be at least 6 characters")
	}

	var hash *string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE id = $1", userID).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.New("user not found")
		}
		return err
	}
	if hash == nil {
		return errors.New("this account uses social login and has no password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*hash), []byte(req.CurrentPassword)); err != nil {
		return errors.New("current password is incorrect")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		"UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1",
		userID, string(newHash),
	)
	return err
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

// LoginWithVKOAuth finds or creates a user for the given VK user ID and returns JWT tokens.
func (s *AuthService) LoginWithVKOAuth(vkUserID int64, firstName, lastName, accessToken string) (string, string, error) {
	providerUserID := fmt.Sprintf("%d", vkUserID)
	email := fmt.Sprintf("vk_%d@vk.inbetwin.local", vkUserID)

	// Try to find existing user via oauth_providers
	var userID string
	err := s.db.QueryRow(`
		SELECT user_id FROM oauth_providers
		WHERE provider = 'vk' AND provider_user_id = $1`, providerUserID).Scan(&userID)

	if err != nil {
		// Create new user
		err = s.db.QueryRow(`
			INSERT INTO users (email, password_hash, first_name, last_name)
			VALUES ($1, NULL, $2, $3)
			ON CONFLICT (email) DO UPDATE SET first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name
			RETURNING id`, email, firstName, lastName).Scan(&userID)
		if err != nil {
			return "", "", fmt.Errorf("create user: %w", err)
		}
		// Save oauth_providers row
		_, err = s.db.Exec(`
			INSERT INTO oauth_providers (user_id, provider, provider_user_id, access_token)
			VALUES ($1, 'vk', $2, $3)
			ON CONFLICT (provider, provider_user_id) DO UPDATE SET access_token = EXCLUDED.access_token`,
			userID, providerUserID, accessToken)
		if err != nil {
			return "", "", fmt.Errorf("save oauth provider: %w", err)
		}
	} else {
		// Update token
		_, _ = s.db.Exec(`UPDATE oauth_providers SET access_token = $1
			WHERE provider = 'vk' AND provider_user_id = $2`, accessToken, providerUserID)
	}

	// Issue JWT
	parsedID := uuid.MustParse(userID)
	accessJWT, err := s.jwtService.GenerateAccessToken(parsedID, email, "free")
	if err != nil {
		return "", "", err
	}
	refreshJWT, err := s.jwtService.GenerateRefreshToken(parsedID)
	if err != nil {
		return "", "", err
	}
	if err := s.saveRefreshToken(parsedID, refreshJWT, "vk-oauth"); err != nil {
		return "", "", err
	}
	return accessJWT, refreshJWT, nil
}

func stringPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
