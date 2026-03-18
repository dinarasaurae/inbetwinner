package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
	"github.com/dinarasaurae/inbetwin-social-service/internal/crypto"
	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
	vkapi "github.com/dinarasaurae/inbetwin-social-service/internal/vk"
)

// ─── OAuth state store ────────────────────────────────────────────────────────

type pendingOAuth struct {
	userID  uuid.UUID
	groupID int64
	expiry  time.Time
}

// ─── VKService ────────────────────────────────────────────────────────────────

// VKService handles VK group integration lifecycle.
type VKService struct {
	db  *database.DB
	enc *crypto.Encryptor
	cfg *config.Config

	// oauthStates stores short-lived CSRF nonces for the OAuth flow.
	oauthMu     sync.Mutex
	oauthStates map[string]pendingOAuth

	// workers tracks running Long Poll goroutines per integration ID.
	workerMu      sync.Mutex
	workerCancels map[uuid.UUID]context.CancelFunc
}

// NewVKService creates a new VKService.
func NewVKService(db *database.DB, enc *crypto.Encryptor, cfg *config.Config) *VKService {
	svc := &VKService{
		db:            db,
		enc:           enc,
		cfg:           cfg,
		oauthStates:   make(map[string]pendingOAuth),
		workerCancels: make(map[uuid.UUID]context.CancelFunc),
	}
	// Periodically clean up expired OAuth states.
	go svc.cleanExpiredStates()
	return svc
}

func (s *VKService) cleanExpiredStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.oauthMu.Lock()
		for k, v := range s.oauthStates {
			if v.expiry.Before(now) {
				delete(s.oauthStates, k)
			}
		}
		s.oauthMu.Unlock()
	}
}

// ─── OAuth flow ───────────────────────────────────────────────────────────────

// OAuthStart generates a VK authorization URL for the mobile app to open.
// groupID: the VK group ID the user wants to connect.
// Returns auth_url and a CSRF state nonce.
func (s *VKService) OAuthStart(ctx context.Context, userID uuid.UUID, groupID int64) (string, string, error) {
	if s.cfg.VKAppID == "" {
		return "", "", fmt.Errorf("VK_APP_ID not configured")
	}

	// Generate a random state nonce (CSRF protection).
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate state: %w", err)
	}
	state := hex.EncodeToString(b)

	s.oauthMu.Lock()
	s.oauthStates[state] = pendingOAuth{
		userID:  userID,
		groupID: groupID,
		expiry:  time.Now().Add(10 * time.Minute),
	}
	s.oauthMu.Unlock()

	// Scopes needed: messages (DM read/write), wall (post import), photos
	// offline = no expiry on the token
	authURL := fmt.Sprintf(
		"%s/authorize?client_id=%s&display=mobile&redirect_uri=%s"+
			"&scope=messages,wall,photos,offline&response_type=code&v=%s"+
			"&group_ids=%d&state=%s",
		vkapi.OAuthBase,
		url.QueryEscape(s.cfg.VKAppID),
		url.QueryEscape(s.cfg.VKRedirectURI),
		vkapi.APIVersion,
		groupID,
		state,
	)

	return authURL, state, nil
}

// OAuthExchange exchanges the authorization code for a group token and stores the integration.
func (s *VKService) OAuthExchange(ctx context.Context, code, state string) (*models.VKIntegration, error) {
	// Validate state.
	s.oauthMu.Lock()
	pending, ok := s.oauthStates[state]
	if ok {
		delete(s.oauthStates, state)
	}
	s.oauthMu.Unlock()

	if !ok || pending.expiry.Before(time.Now()) {
		return nil, fmt.Errorf("invalid_state: OAuth state not found or expired")
	}

	// Exchange code → token.
	tokenResp, err := s.exchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token_exchange: %w", err)
	}

	// Extract the group token (key: "access_token_{group_id}").
	groupToken, err := extractGroupToken(tokenResp, pending.groupID)
	if err != nil {
		return nil, err
	}

	// Fetch group metadata.
	userClient := vkapi.NewUserClient(tokenResp["access_token"])
	group, err := userClient.GroupsGetByID(ctx, pending.groupID)
	if err != nil {
		return nil, fmt.Errorf("fetch_group: %w", err)
	}

	// Encrypt the group token.
	var tokenEnc, tokenIV []byte
	if s.enc != nil {
		tokenEnc, tokenIV, err = s.enc.Encrypt([]byte(groupToken))
		if err != nil {
			return nil, fmt.Errorf("encrypt_token: %w", err)
		}
	} else {
		tokenEnc = []byte(groupToken)
		tokenIV = []byte("DEV_NO_IV_TWELVE!")
	}

	// Upsert the integration.
	integ := &models.VKIntegration{
		ID:              uuid.New(),
		UserID:          pending.userID,
		GroupID:         pending.groupID,
		GroupTokenEnc:   tokenEnc,
		GroupTokenIV:    tokenIV,
		GroupName:       group.Name,
		GroupScreenName: group.ScreenName,
		GroupPhoto:      group.Photo200,
		IsActive:        true,
		ConnectedAt:     time.Now(),
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO vk_integrations
			(id, user_id, group_id, group_token_enc, group_token_iv,
			 group_name, group_screen_name, group_photo, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,TRUE)
		ON CONFLICT (user_id, group_id) DO UPDATE SET
			group_token_enc   = EXCLUDED.group_token_enc,
			group_token_iv    = EXCLUDED.group_token_iv,
			group_name        = EXCLUDED.group_name,
			group_screen_name = EXCLUDED.group_screen_name,
			group_photo       = EXCLUDED.group_photo,
			is_active         = TRUE`,
		integ.ID, integ.UserID, integ.GroupID,
		integ.GroupTokenEnc, integ.GroupTokenIV,
		integ.GroupName, integ.GroupScreenName, integ.GroupPhoto,
	)
	if err != nil {
		return nil, fmt.Errorf("save_integration: %w", err)
	}

	// Re-fetch to get the actual UUID (ON CONFLICT may have kept the old ID).
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM vk_integrations WHERE user_id=$1 AND group_id=$2`,
		integ.UserID, integ.GroupID,
	).Scan(&integ.ID); err != nil {
		return nil, err
	}

	// Start the Long Poll worker for this group.
	go s.startWorker(context.Background(), integ, groupToken)

	return integ, nil
}

// exchangeCode calls the VK OAuth token endpoint and returns the raw response map.
func (s *VKService) exchangeCode(ctx context.Context, code string) (map[string]string, error) {
	params := url.Values{
		"client_id":     {s.cfg.VKAppID},
		"client_secret": {s.cfg.VKAppSecret},
		"redirect_uri":  {s.cfg.VKRedirectURI},
		"code":          {code},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		vkapi.OAuthBase+"/access_token",
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]json.RawMessage
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	// Check for OAuth error.
	if errField, ok := result["error"]; ok {
		var errMsg string
		_ = json.Unmarshal(errField, &errMsg)
		return nil, fmt.Errorf("vk oauth error: %s", errMsg)
	}

	// Convert all string values to a flat map.
	flat := make(map[string]string)
	for k, v := range result {
		var sv string
		if json.Unmarshal(v, &sv) == nil {
			flat[k] = sv
		} else {
			// It might be a number; store as string.
			flat[k] = string(v)
		}
	}
	return flat, nil
}

// extractGroupToken finds the group access token in the OAuth response map.
// VK returns it as "access_token_{group_id}".
func extractGroupToken(resp map[string]string, groupID int64) (string, error) {
	key := fmt.Sprintf("access_token_%d", groupID)
	if t, ok := resp[key]; ok && t != "" {
		return t, nil
	}
	// Fallback: some VK app configurations return a single access_token for the group.
	if t, ok := resp["access_token"]; ok && t != "" {
		return t, nil
	}
	return "", fmt.Errorf("group_token_missing: no token found for group %d in OAuth response", groupID)
}

// ─── Integration management ───────────────────────────────────────────────────

// ListIntegrations returns all active VK integrations for a user.
func (s *VKService) ListIntegrations(ctx context.Context, userID uuid.UUID) ([]*models.VKIntegration, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, group_id, group_name, group_screen_name, group_photo,
		       is_active, connected_at
		FROM vk_integrations
		WHERE user_id = $1 AND is_active = TRUE
		ORDER BY connected_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.VKIntegration
	for rows.Next() {
		i := &models.VKIntegration{}
		if err := rows.Scan(&i.ID, &i.UserID, &i.GroupID,
			&i.GroupName, &i.GroupScreenName, &i.GroupPhoto,
			&i.IsActive, &i.ConnectedAt); err != nil {
			return nil, err
		}
		list = append(list, i)
	}
	return list, rows.Err()
}

// Disconnect deactivates a VK integration and stops its Long Poll worker.
func (s *VKService) Disconnect(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE vk_integrations SET is_active = FALSE
		WHERE id = $1 AND user_id = $2`, integrationID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("not_found: integration not found or unauthorized")
	}
	s.stopWorker(integrationID)
	return nil
}

// ─── Wall post sync ───────────────────────────────────────────────────────────

// SyncPosts imports wall posts from the VK group's wall.
// Returns the number of new posts stored.
func (s *VKService) SyncPosts(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID, count int) (int, error) {
	if count <= 0 || count > 100 {
		count = 50
	}

	integ, groupToken, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return 0, err
	}

	client := vkapi.NewClient(groupToken, integ.GroupID)
	ownerID := -integ.GroupID // negative = group

	result, err := client.WallGet(ctx, ownerID, count, 0)
	if err != nil {
		return 0, fmt.Errorf("wall.get: %w", err)
	}

	imported := 0
	for _, p := range result.Items {
		post := buildVKPost(p, integrationID, userID)
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO vk_posts
				(id, integration_id, user_id, vk_post_id, owner_id, text,
				 media_type, has_media, likes_count, reposts_count,
				 views_count, comments_count, extracted_urls, posted_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (integration_id, vk_post_id) DO NOTHING`,
			post.ID, post.IntegrationID, post.UserID, post.VKPostID, post.OwnerID,
			post.Text, post.MediaType, post.HasMedia,
			post.LikesCount, post.RepostsCount, post.ViewsCount, post.CommentsCount,
			pq.Array(post.ExtractedURLs), post.PostedAt,
		)
		if err != nil {
			log.Printf("[vk] sync: insert post %d: %v", p.ID, err)
			continue
		}
		imported++
	}
	return imported, nil
}

// ─── Messages ─────────────────────────────────────────────────────────────────

// SendMessage sends a text message to a lead from the VK group.
func (s *VKService) SendMessage(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID, toVKUserID int64, text string) error {
	integ, groupToken, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return err
	}

	// Human-like delay.
	humanDelayVK(ctx)

	client := vkapi.NewClient(groupToken, integ.GroupID)
	msgID, err := client.MessagesSend(ctx, toVKUserID, text)
	if err != nil {
		return fmt.Errorf("messages.send: %w", err)
	}

	// Persist the outgoing message.
	msgIDVal := msgID
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO vk_messages
			(integration_id, from_vk_user_id, message_id, text, is_incoming, is_processed)
		VALUES ($1,$2,$3,$4,FALSE,TRUE)
		ON CONFLICT DO NOTHING`,
		integrationID, toVKUserID, msgIDVal, text,
	)
	return nil
}

// ─── Lead enrichment ──────────────────────────────────────────────────────────

// EnrichLead fetches the VK user's public profile and persists it as a digital twin.
func (s *VKService) EnrichLead(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID, vkUserID int64) (*models.VKLeadProfile, error) {
	integ, groupToken, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return nil, err
	}
	_ = integ

	client := vkapi.NewClient(groupToken, 0)
	users, err := client.UsersGet(ctx, []int64{vkUserID})
	if err != nil || len(users) == 0 {
		return nil, fmt.Errorf("users.get: %w", err)
	}
	u := users[0]

	profile := &models.VKLeadProfile{
		ID:             uuid.New(),
		IntegrationID:  integrationID,
		VKUserID:       vkUserID,
		FirstName:      u.FirstName,
		LastName:       u.LastName,
		Sex:            u.Sex,
		BDate:          u.BDate,
		Domain:         u.Domain,
		About:          u.About,
		Status:         u.Status,
		PhotoURL:       u.Photo200,
		FollowersCount: u.FollowersCount,
	}
	if u.City != nil {
		profile.City = u.City.Title
	}
	if u.Country != nil {
		profile.Country = u.Country.Title
	}
	if u.Occupation != nil {
		profile.OccupationType = u.Occupation.Type
		profile.OccupationName = u.Occupation.Name
	}
	now := time.Now()
	profile.LastEnrichedAt = &now

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO vk_lead_profiles
			(id, integration_id, vk_user_id, first_name, last_name, sex,
			 bdate, city, country, about, status, domain, photo_url,
			 followers_count, occupation_type, occupation_name, last_enriched_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (integration_id, vk_user_id) DO UPDATE SET
			first_name       = EXCLUDED.first_name,
			last_name        = EXCLUDED.last_name,
			sex              = EXCLUDED.sex,
			bdate            = EXCLUDED.bdate,
			city             = EXCLUDED.city,
			country          = EXCLUDED.country,
			about            = EXCLUDED.about,
			status           = EXCLUDED.status,
			domain           = EXCLUDED.domain,
			photo_url        = EXCLUDED.photo_url,
			followers_count  = EXCLUDED.followers_count,
			occupation_type  = EXCLUDED.occupation_type,
			occupation_name  = EXCLUDED.occupation_name,
			last_enriched_at = EXCLUDED.last_enriched_at`,
		profile.ID, profile.IntegrationID, profile.VKUserID,
		profile.FirstName, profile.LastName, profile.Sex,
		nullStr(profile.BDate), nullStr(profile.City), nullStr(profile.Country),
		nullStr(profile.About), nullStr(profile.Status), nullStr(profile.Domain),
		nullStr(profile.PhotoURL), profile.FollowersCount,
		nullStr(profile.OccupationType), nullStr(profile.OccupationName),
		profile.LastEnrichedAt,
	)
	return profile, err
}

// ─── Long Poll worker management ──────────────────────────────────────────────

// StartAllWorkers loads all active VK integrations and starts a Long Poll worker
// for each. Called once on service startup.
func (s *VKService) StartAllWorkers(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, group_id, group_token_enc, group_token_iv,
		       group_name, group_screen_name, group_photo, long_poll_ts
		FROM vk_integrations WHERE is_active = TRUE`)
	if err != nil {
		log.Printf("[vk] StartAllWorkers: query: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		i := &models.VKIntegration{}
		if err := rows.Scan(
			&i.ID, &i.UserID, &i.GroupID,
			&i.GroupTokenEnc, &i.GroupTokenIV,
			&i.GroupName, &i.GroupScreenName, &i.GroupPhoto,
			&i.LongPollTs,
		); err != nil {
			log.Printf("[vk] StartAllWorkers: scan: %v", err)
			continue
		}

		groupToken, err := s.decryptToken(i.GroupTokenEnc, i.GroupTokenIV)
		if err != nil {
			log.Printf("[vk] StartAllWorkers: decrypt token for %s: %v", i.ID, err)
			continue
		}
		go s.startWorker(ctx, i, groupToken)
	}
}

func (s *VKService) startWorker(ctx context.Context, integ *models.VKIntegration, groupToken string) {
	workerCtx, cancel := context.WithCancel(ctx)

	s.workerMu.Lock()
	if old, ok := s.workerCancels[integ.ID]; ok {
		old() // stop old worker if any
	}
	s.workerCancels[integ.ID] = cancel
	s.workerMu.Unlock()

	client := vkapi.NewClient(groupToken, integ.GroupID)
	integID := integ.ID // capture
	initialTs := integ.LongPollTs

	worker := vkapi.NewWorker(
		client,
		integ.GroupID,
		s.handleIncomingMessage,
		func() string { return initialTs },
		func(ts string) { s.persistLPTs(integID, ts) },
	)
	worker.Run(workerCtx)
}

func (s *VKService) stopWorker(integrationID uuid.UUID) {
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	if cancel, ok := s.workerCancels[integrationID]; ok {
		cancel()
		delete(s.workerCancels, integrationID)
	}
}

// handleIncomingMessage is called by the Long Poll worker on each message_new event.
func (s *VKService) handleIncomingMessage(ctx context.Context, groupID int64, msg vkapi.IncomingMessage) {
	// Find the integration for this group.
	var integID uuid.UUID
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM vk_integrations WHERE group_id=$1 AND is_active=TRUE LIMIT 1`,
		groupID,
	).Scan(&integID)
	if err != nil {
		log.Printf("[vk-lp] group %d: integration not found: %v", groupID, err)
		return
	}

	text := &msg.Text
	if msg.Text == "" {
		text = nil
	}
	convMsgID := msg.ConversationMessageID

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO vk_messages
			(integration_id, from_vk_user_id, message_id,
			 conversation_message_id, text, is_incoming, is_processed)
		VALUES ($1,$2,$3,$4,$5,TRUE,FALSE)
		ON CONFLICT (integration_id, message_id) DO NOTHING`,
		integID, msg.FromID, msg.ID,
		nullInt64(convMsgID), text,
	)
	if err != nil {
		log.Printf("[vk-lp] group %d: save message: %v", groupID, err)
		return
	}

	// Asynchronously enrich the lead profile if not seen before.
	go func() {
		enCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var exists bool
		_ = s.db.QueryRowContext(enCtx,
			`SELECT EXISTS(SELECT 1 FROM vk_lead_profiles WHERE integration_id=$1 AND vk_user_id=$2)`,
			integID, msg.FromID,
		).Scan(&exists)

		if !exists {
			// Load userID for this integration.
			var ownerUserID uuid.UUID
			if err := s.db.QueryRowContext(enCtx,
				`SELECT user_id FROM vk_integrations WHERE id=$1`, integID,
			).Scan(&ownerUserID); err == nil {
				if _, err := s.EnrichLead(enCtx, ownerUserID, integID, msg.FromID); err != nil {
					log.Printf("[vk-lp] enrich lead %d: %v", msg.FromID, err)
				}
			}
		}
	}()
}

func (s *VKService) persistLPTs(integID uuid.UUID, ts string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.db.ExecContext(ctx,
		`UPDATE vk_integrations SET long_poll_ts=$1 WHERE id=$2`, ts, integID)
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// loadIntegration fetches the integration and decrypts its group token.
func (s *VKService) loadIntegration(ctx context.Context, userID, integrationID uuid.UUID) (*models.VKIntegration, string, error) {
	i := &models.VKIntegration{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, group_id, group_token_enc, group_token_iv,
		       group_name, group_screen_name, long_poll_ts
		FROM vk_integrations
		WHERE id=$1 AND user_id=$2 AND is_active=TRUE`,
		integrationID, userID,
	).Scan(&i.ID, &i.UserID, &i.GroupID,
		&i.GroupTokenEnc, &i.GroupTokenIV,
		&i.GroupName, &i.GroupScreenName, &i.LongPollTs)
	if err != nil {
		return nil, "", fmt.Errorf("not_found: %w", err)
	}
	token, err := s.decryptToken(i.GroupTokenEnc, i.GroupTokenIV)
	return i, token, err
}

func (s *VKService) decryptToken(enc, iv []byte) (string, error) {
	if s.enc == nil {
		return string(enc), nil
	}
	plain, err := s.enc.Decrypt(enc, iv)
	if err != nil {
		return "", fmt.Errorf("decrypt_token: %w", err)
	}
	return string(plain), nil
}

// buildVKPost converts a vkapi.WallPost to models.VKPost.
func buildVKPost(p vkapi.WallPost, integrationID, userID uuid.UUID) *models.VKPost {
	post := &models.VKPost{
		ID:            uuid.New(),
		IntegrationID: integrationID,
		UserID:        userID,
		VKPostID:      p.ID,
		OwnerID:       p.OwnerID,
		PostedAt:      time.Unix(p.Date, 0).UTC(),
	}
	if p.Text != "" {
		post.Text = &p.Text
	}
	if len(p.Attachments) > 0 {
		post.HasMedia = true
		t := p.Attachments[0].Type
		post.MediaType = &t
	}
	likes := p.Likes.Count
	reposts := p.Reposts.Count
	views := p.Views.Count
	comments := p.Comments.Count
	post.LikesCount = &likes
	post.RepostsCount = &reposts
	if views > 0 {
		post.ViewsCount = &views
	}
	post.CommentsCount = &comments
	return post
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullInt64(n int64) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

// humanDelayVK adds a 1–3 s random pause to avoid looking like an instant bot.
func humanDelayVK(ctx context.Context) {
	jitter := time.Duration(1000+time.Now().UnixNano()%2000) * time.Millisecond
	select {
	case <-time.After(jitter):
	case <-ctx.Done():
	}
}
