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

	var email, subscriptionPlan string
	err = s.db.QueryRow("SELECT email, subscription_plan FROM users WHERE id = $1",
		claims.UserID).Scan(&email, &subscriptionPlan)
	if err != nil {
		return "", err
	}

	tokenHash := s.jwtService.HashToken(refreshToken)
	var exists bool
	err = s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE token_hash = $1 AND user_id = $2)",
		tokenHash, claims.UserID).Scan(&exists)
	if err != nil {
		return "", err
	}
	if !exists {
		// Recover valid refresh tokens after auth DB resets or OAuth relinking, where
		// the signed JWT is still valid but its hash row was lost.
		if err := s.saveRefreshToken(claims.UserID, refreshToken, "recovered-refresh"); err != nil {
			return "", err
		}
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
func (s *AuthService) LoginWithVKOAuth(vkUserID int64, firstName, lastName, accessToken string, preferredUserID *uuid.UUID) (string, string, error) {
	providerUserID := fmt.Sprintf("%d", vkUserID)
	email := fmt.Sprintf("vk_%d@vk.inbetwin.local", vkUserID)
	firstNamePtr := nullableTrimmedString(firstName)
	lastNamePtr := nullableTrimmedString(lastName)
	namePtr := buildDisplayName(firstNamePtr, lastNamePtr)
	plan := "free"

	tx, err := s.db.Begin()
	if err != nil {
		return "", "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Try to find existing user via oauth_providers
	var linkedUserID uuid.UUID
	providerExists := false
	err = tx.QueryRow(`
		SELECT user_id FROM oauth_providers
		WHERE provider = 'vk' AND provider_user_id = $1`, providerUserID).Scan(&linkedUserID)
	if err != nil && err != sql.ErrNoRows {
		return "", "", fmt.Errorf("load oauth provider: %w", err)
	}
	if err == nil {
		providerExists = true
	}

	var canonicalUserID uuid.UUID
	var orphanedUserIDs []uuid.UUID

	switch {
	case preferredUserID != nil:
		canonicalUserID = *preferredUserID
		orphanedUserIDs, plan, err = s.preparePreferredVKUserTx(tx, canonicalUserID, linkedUserID, providerExists, email)
		if err != nil {
			return "", "", fmt.Errorf("prepare preferred vk user: %w", err)
		}
		plan, err = s.upsertUserTx(tx, canonicalUserID, email, namePtr, firstNamePtr, lastNamePtr, plan)
		if err != nil {
			return "", "", fmt.Errorf("upsert preferred user: %w", err)
		}
	case providerExists:
		canonicalUserID = linkedUserID
		plan, err = s.upsertUserTx(tx, canonicalUserID, email, namePtr, firstNamePtr, lastNamePtr, plan)
		if err != nil {
			return "", "", fmt.Errorf("refresh existing vk user: %w", err)
		}
	default:
		canonicalUserID, plan, err = s.findOrCreateOAuthUserByEmailTx(tx, email, namePtr, firstNamePtr, lastNamePtr, plan)
		if err != nil {
			return "", "", fmt.Errorf("create oauth user: %w", err)
		}
	}

	if err := s.ensureSubscriptionTx(tx, canonicalUserID, plan); err != nil {
		return "", "", fmt.Errorf("ensure subscription: %w", err)
	}

	if err := s.upsertVKProviderTx(tx, canonicalUserID, providerUserID, accessToken); err != nil {
		return "", "", fmt.Errorf("save oauth provider: %w", err)
	}

	for _, orphanUserID := range orphanedUserIDs {
		if orphanUserID == canonicalUserID {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM users WHERE id = $1`, orphanUserID); err != nil {
			return "", "", fmt.Errorf("delete orphaned vk user %s: %w", orphanUserID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("commit vk login: %w", err)
	}

	// Issue JWT
	accessJWT, err := s.jwtService.GenerateAccessToken(canonicalUserID, email, plan)
	if err != nil {
		return "", "", err
	}
	refreshJWT, err := s.jwtService.GenerateRefreshToken(canonicalUserID)
	if err != nil {
		return "", "", err
	}
	if err := s.saveRefreshToken(canonicalUserID, refreshJWT, "vk-oauth"); err != nil {
		return "", "", err
	}
	return accessJWT, refreshJWT, nil
}

func (s *AuthService) findOrCreateOAuthUserByEmailTx(
	tx *sql.Tx,
	email string,
	name, firstName, lastName *string,
	plan string,
) (uuid.UUID, string, error) {
	var userID uuid.UUID
	actualPlan := plan

	err := tx.QueryRow(`
		INSERT INTO users (email, password_hash, name, first_name, last_name, email_verified, subscription_plan, created_at, updated_at)
		VALUES ($1, NULL, $2, $3, $4, FALSE, $5, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE
		SET
			name = COALESCE(EXCLUDED.name, users.name),
			first_name = COALESCE(EXCLUDED.first_name, users.first_name),
			last_name = COALESCE(EXCLUDED.last_name, users.last_name),
			updated_at = NOW()
		RETURNING id, subscription_plan
	`, email, name, firstName, lastName, plan).Scan(&userID, &actualPlan)
	if err != nil {
		return uuid.Nil, "", err
	}

	return userID, actualPlan, nil
}

func (s *AuthService) upsertUserTx(
	tx *sql.Tx,
	userID uuid.UUID,
	email string,
	name, firstName, lastName *string,
	plan string,
) (string, error) {
	actualPlan := plan

	err := tx.QueryRow(`
		INSERT INTO users (id, email, password_hash, name, first_name, last_name, email_verified, subscription_plan, created_at, updated_at)
		VALUES ($1, $2, NULL, $3, $4, $5, FALSE, $6, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE
		SET
			email = EXCLUDED.email,
			name = COALESCE(EXCLUDED.name, users.name),
			first_name = COALESCE(EXCLUDED.first_name, users.first_name),
			last_name = COALESCE(EXCLUDED.last_name, users.last_name),
			updated_at = NOW()
		RETURNING subscription_plan
	`, userID, email, name, firstName, lastName, plan).Scan(&actualPlan)
	if err != nil {
		return "", err
	}

	return actualPlan, nil
}

func (s *AuthService) preparePreferredVKUserTx(
	tx *sql.Tx,
	canonicalUserID uuid.UUID,
	linkedUserID uuid.UUID,
	providerExists bool,
	email string,
) ([]uuid.UUID, string, error) {
	plan := "free"
	orphanedUserIDs := make([]uuid.UUID, 0, 2)
	seen := map[uuid.UUID]struct{}{}

	addOrphan := func(userID uuid.UUID) error {
		if userID == uuid.Nil || userID == canonicalUserID {
			return nil
		}
		if _, ok := seen[userID]; ok {
			return nil
		}

		var userPlan string
		err := tx.QueryRow(`SELECT subscription_plan FROM users WHERE id = $1`, userID).Scan(&userPlan)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}
		if userPlan != "" {
			plan = userPlan
		}

		_, err = tx.Exec(`UPDATE users SET email = $2, updated_at = NOW() WHERE id = $1`, userID, orphanedVKEmail(userID))
		if err != nil {
			return err
		}

		seen[userID] = struct{}{}
		orphanedUserIDs = append(orphanedUserIDs, userID)
		return nil
	}

	if providerExists {
		if err := addOrphan(linkedUserID); err != nil {
			return nil, "", err
		}
	}

	var emailUserID uuid.UUID
	err := tx.QueryRow(`SELECT id FROM users WHERE email = $1`, email).Scan(&emailUserID)
	if err != nil && err != sql.ErrNoRows {
		return nil, "", err
	}
	if err == nil {
		if err := addOrphan(emailUserID); err != nil {
			return nil, "", err
		}
	}

	return orphanedUserIDs, plan, nil
}

func (s *AuthService) ensureSubscriptionTx(tx *sql.Tx, userID uuid.UUID, plan string) error {
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM subscriptions WHERE user_id = $1)`, userID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	now := time.Now()
	_, err := tx.Exec(`
		INSERT INTO subscriptions (id, user_id, plan, status, started_at, trial_ends_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		uuid.New(),
		userID,
		plan,
		"trialing",
		now,
		now.Add(7*24*time.Hour),
		now,
		now,
	)
	return err
}

func (s *AuthService) upsertVKProviderTx(tx *sql.Tx, userID uuid.UUID, providerUserID, accessToken string) error {
	_, err := tx.Exec(`
		INSERT INTO oauth_providers (user_id, provider, provider_user_id, access_token)
		VALUES ($1, 'vk', $2, $3)
		ON CONFLICT (provider, provider_user_id) DO UPDATE
		SET
			user_id = EXCLUDED.user_id,
			access_token = EXCLUDED.access_token,
			updated_at = NOW()
	`, userID, providerUserID, accessToken)
	return err
}

func nullableTrimmedString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func buildDisplayName(firstName, lastName *string) *string {
	parts := make([]string, 0, 2)
	if firstName != nil {
		parts = append(parts, *firstName)
	}
	if lastName != nil {
		parts = append(parts, *lastName)
	}

	fullName := strings.TrimSpace(strings.Join(parts, " "))
	if fullName == "" {
		return nil
	}

	return &fullName
}

func orphanedVKEmail(userID uuid.UUID) string {
	return fmt.Sprintf("orphaned+%s@vk.inbetwin.local", userID)
}

func stringPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
