package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/google/uuid"
)

// DigitalTwin is the unified, enriched profile of a lead / chat user.
// It is built from the platform's own profile API (VK users.get, Telegram
// user object, Facebook PSID) and optionally enriched with Pinterest data.
//
// The twin is serialised and sent to llm-service so the agent can personalise
// every response: use the person's name, reference their city or occupation,
// mention topics that match their Pinterest interests.
type DigitalTwin struct {
	Platform    string `json:"platform"`
	ChatUserID  string `json:"chat_user_id"`

	// Identity
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	City      string `json:"city,omitempty"`
	Country   string `json:"country,omitempty"`

	// Occupation
	Company  string `json:"company,omitempty"`
	JobTitle string `json:"job_title,omitempty"`

	// Bio
	Bio    string `json:"bio,omitempty"`
	Status string `json:"status,omitempty"`

	// Social signals
	FollowersCount int `json:"followers_count,omitempty"`

	// Pinterest
	PinterestURL    string   `json:"pinterest_url,omitempty"`
	PinterestBoards []string `json:"pinterest_boards,omitempty"` // board names = interests

	// Facebook Messenger
	FacebookName string `json:"facebook_name,omitempty"`

	// Behavioural
	MessageCount int `json:"message_count,omitempty"`
	LeadScore    int `json:"lead_score,omitempty"`
}

// FullName returns "FirstName LastName" trimmed, or empty string.
func (t *DigitalTwin) FullName() string {
	return strings.TrimSpace(t.FirstName + " " + t.LastName)
}

// SummaryForLLM formats the twin as a compact, human-readable block that is
// injected into the agent's system prompt.  Omits empty / zero fields so the
// prompt stays concise.
func (t *DigitalTwin) SummaryForLLM() string {
	if t == nil {
		return ""
	}
	sb := strings.Builder{}
	sb.WriteString("--- Профиль клиента ---\n")

	if name := t.FullName(); name != "" {
		sb.WriteString("Имя: " + name + "\n")
	}

	loc := strings.Join(nonEmpty(t.City, t.Country), ", ")
	if loc != "" {
		sb.WriteString("Местоположение: " + loc + "\n")
	}

	if t.Company != "" || t.JobTitle != "" {
		occ := strings.Join(nonEmpty(t.JobTitle, t.Company), " в ")
		sb.WriteString("Работа: " + occ + "\n")
	}

	if t.Bio != "" {
		bio := t.Bio
		if len(bio) > 200 {
			bio = bio[:197] + "..."
		}
		sb.WriteString("О себе: " + bio + "\n")
	}

	if t.Status != "" {
		sb.WriteString("Статус: \"" + t.Status + "\"\n")
	}

	if t.FollowersCount > 0 {
		sb.WriteString(fmt.Sprintf("Подписчиков: %d\n", t.FollowersCount))
	}

	if len(t.PinterestBoards) > 0 {
		boards := t.PinterestBoards
		if len(boards) > 6 {
			boards = boards[:6]
		}
		sb.WriteString("Pinterest интересы: " + strings.Join(boards, ", ") + "\n")
	} else if t.PinterestURL != "" {
		sb.WriteString("Pinterest: " + t.PinterestURL + "\n")
	}

	if t.FacebookName != "" {
		sb.WriteString("Facebook: " + t.FacebookName + "\n")
	}

	if t.LeadScore > 0 {
		sb.WriteString(fmt.Sprintf("Оценка лида: %d/100\n", t.LeadScore))
	}

	sb.WriteString("--- Конец профиля ---")
	return sb.String()
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// ── In-memory cache ───────────────────────────────────────────────────────────

const twinCacheTTL = time.Hour

type cachedTwin struct {
	twin    *DigitalTwin
	expires time.Time
}

type twinCache struct {
	mu    sync.Mutex
	items map[string]*cachedTwin
}

func newTwinCache() *twinCache { return &twinCache{items: map[string]*cachedTwin{}} }

func (c *twinCache) key(workspaceID uuid.UUID, platform, chatUserID string) string {
	return workspaceID.String() + "|" + platform + "|" + chatUserID
}

func (c *twinCache) get(workspaceID uuid.UUID, platform, chatUserID string) *DigitalTwin {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[c.key(workspaceID, platform, chatUserID)]
	if !ok || time.Now().After(item.expires) {
		return nil
	}
	return item.twin
}

func (c *twinCache) set(workspaceID uuid.UUID, platform, chatUserID string, twin *DigitalTwin) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[c.key(workspaceID, platform, chatUserID)] = &cachedTwin{
		twin:    twin,
		expires: time.Now().Add(twinCacheTTL),
	}
}

// ── DigitalTwinService ────────────────────────────────────────────────────────

// DigitalTwinService builds, caches, and persists digital twins.
// It is the single source of truth for the enriched lead profile
// that gets injected into every agent system prompt.
type DigitalTwinService struct {
	db        *database.DB
	pinterest *PinterestClient
	facebook  *FacebookClient
	cache     *twinCache
}

func NewDigitalTwinService(db *database.DB, pinterest *PinterestClient, facebook *FacebookClient) *DigitalTwinService {
	return &DigitalTwinService{
		db:        db,
		pinterest: pinterest,
		facebook:  facebook,
		cache:     newTwinCache(),
	}
}

// GetOrBuild returns the cached twin for (workspaceID, platform, chatUserID).
// If not cached, loads from DB and asynchronously triggers enrichment if stale.
// Always returns fast (cache-first, no blocking external calls).
func (s *DigitalTwinService) GetOrBuild(ctx context.Context, workspaceID uuid.UUID, platform, chatUserID string) *DigitalTwin {
	if cached := s.cache.get(workspaceID, platform, chatUserID); cached != nil {
		return cached
	}
	twin, err := s.loadFromDB(ctx, workspaceID, platform, chatUserID)
	if err != nil {
		log.Printf("[twin] loadFromDB %s/%s: %v", platform, chatUserID, err)
		return nil
	}
	if twin != nil {
		s.cache.set(workspaceID, platform, chatUserID, twin)
		// If stale (>1h), re-enrich in background.
		if twin.PinterestURL != "" && len(twin.PinterestBoards) == 0 && s.pinterest.Configured() {
			go s.enrichPinterestAsync(workspaceID, platform, chatUserID, twin)
		}
	}
	return twin
}

// UpsertFromVKProfile creates or updates the digital twin from a VK lead profile
// row.  Called by vk.go after EnrichLead succeeds.
func (s *DigitalTwinService) UpsertFromVKProfile(ctx context.Context,
	workspaceID uuid.UUID,
	chatUserID string, // stringified vk_user_id
	firstName, lastName, city, country, bio, status, occupationType, occupationName string,
	followersCount int,
) *DigitalTwin {
	twin := &DigitalTwin{
		Platform:       "vk",
		ChatUserID:     chatUserID,
		FirstName:      firstName,
		LastName:       lastName,
		City:           city,
		Country:        country,
		Bio:            bio,
		Status:         status,
		FollowersCount: followersCount,
	}

	// Map VK occupation fields to Company / JobTitle.
	switch occupationType {
	case "work":
		twin.Company = occupationName
	case "school", "university":
		twin.Company = occupationName
		twin.JobTitle = "Студент"
	default:
		if occupationName != "" {
			twin.Company = occupationName
		}
	}

	// Extract Pinterest URL from bio.
	if username := ExtractPinterestUsername(bio); username != "" {
		twin.PinterestURL = "https://pinterest.com/" + username
	}

	// Persist to DB.
	if err := s.upsertDB(ctx, workspaceID, twin); err != nil {
		log.Printf("[twin] upsert vk %s: %v", chatUserID, err)
	}

	// Enrich Pinterest in background if URL found and client configured.
	if twin.PinterestURL != "" && s.pinterest.Configured() {
		go s.enrichPinterestAsync(workspaceID, "vk", chatUserID, twin)
	}

	s.cache.set(workspaceID, "vk", chatUserID, twin)
	return twin
}

// UpsertFromFacebook creates or updates the twin from a Facebook Messenger PSID.
// Called when a Facebook Messenger message arrives.
func (s *DigitalTwinService) UpsertFromFacebook(ctx context.Context,
	workspaceID uuid.UUID,
	psid string,
) *DigitalTwin {
	twin := &DigitalTwin{
		Platform:   "facebook",
		ChatUserID: psid,
	}

	if s.facebook.Configured() {
		enrichCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		profile, err := s.facebook.GetMessengerProfile(enrichCtx, psid)
		if err != nil {
			log.Printf("[twin] facebook profile %s: %v", psid, err)
		} else if profile != nil {
			twin.FacebookName = profile.Name
			// Facebook sometimes includes first/last in the name field.
			parts := strings.Fields(profile.Name)
			if len(parts) >= 2 {
				twin.FirstName = parts[0]
				twin.LastName = strings.Join(parts[1:], " ")
			} else if len(parts) == 1 {
				twin.FirstName = parts[0]
			}
		}
	}

	if err := s.upsertDB(ctx, workspaceID, twin); err != nil {
		log.Printf("[twin] upsert facebook %s: %v", psid, err)
	}
	s.cache.set(workspaceID, "facebook", psid, twin)
	return twin
}

// enrichPinterestAsync fetches boards for the twin's Pinterest URL and updates
// the DB + cache. Runs in a goroutine — never blocks the message path.
func (s *DigitalTwinService) enrichPinterestAsync(workspaceID uuid.UUID, platform, chatUserID string, twin *DigitalTwin) {
	username := ExtractPinterestUsername(twin.PinterestURL)
	if username == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, boards, err := s.pinterest.GetLeadProfile(ctx, username)
	if err != nil {
		log.Printf("[twin] pinterest enrich %s: %v", username, err)
		return
	}
	if len(boards) == 0 {
		return
	}

	twin.PinterestBoards = boards
	boardsJSON, _ := json.Marshal(boards)

	_, _ = s.db.ExecContext(ctx, `
		UPDATE digital_twins
		SET pinterest_boards = $1, last_enriched_at = NOW(), updated_at = NOW()
		WHERE workspace_id = $2 AND platform = $3 AND chat_user_id = $4`,
		boardsJSON, workspaceID, platform, chatUserID,
	)
	s.cache.set(workspaceID, platform, chatUserID, twin)
	log.Printf("[twin] pinterest enriched %s: %d boards", username, len(boards))
}

// ── DB helpers ────────────────────────────────────────────────────────────────

func (s *DigitalTwinService) upsertDB(ctx context.Context, workspaceID uuid.UUID, t *DigitalTwin) error {
	boardsJSON, _ := json.Marshal(t.PinterestBoards)
	if boardsJSON == nil {
		boardsJSON = []byte("[]")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO digital_twins
			(workspace_id, platform, chat_user_id,
			 first_name, last_name, city, country,
			 company, job_title, bio, status_text,
			 followers_count, pinterest_url, pinterest_boards,
			 facebook_name, last_enriched_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,NOW())
		ON CONFLICT (workspace_id, platform, chat_user_id) DO UPDATE SET
			first_name       = COALESCE(NULLIF(EXCLUDED.first_name,''),    digital_twins.first_name),
			last_name        = COALESCE(NULLIF(EXCLUDED.last_name,''),     digital_twins.last_name),
			city             = COALESCE(NULLIF(EXCLUDED.city,''),          digital_twins.city),
			country          = COALESCE(NULLIF(EXCLUDED.country,''),       digital_twins.country),
			company          = COALESCE(NULLIF(EXCLUDED.company,''),       digital_twins.company),
			job_title        = COALESCE(NULLIF(EXCLUDED.job_title,''),     digital_twins.job_title),
			bio              = COALESCE(NULLIF(EXCLUDED.bio,''),           digital_twins.bio),
			status_text      = COALESCE(NULLIF(EXCLUDED.status_text,''),  digital_twins.status_text),
			followers_count  = GREATEST(EXCLUDED.followers_count,         digital_twins.followers_count),
			pinterest_url    = COALESCE(NULLIF(EXCLUDED.pinterest_url,''), digital_twins.pinterest_url),
			facebook_name    = COALESCE(NULLIF(EXCLUDED.facebook_name,''), digital_twins.facebook_name),
			last_enriched_at = NOW(),
			updated_at       = NOW()`,
		workspaceID, t.Platform, t.ChatUserID,
		nullStr(t.FirstName), nullStr(t.LastName), nullStr(t.City), nullStr(t.Country),
		nullStr(t.Company), nullStr(t.JobTitle), nullStr(t.Bio), nullStr(t.Status),
		t.FollowersCount, nullStr(t.PinterestURL), boardsJSON,
		nullStr(t.FacebookName),
	)
	return err
}

func (s *DigitalTwinService) loadFromDB(ctx context.Context, workspaceID uuid.UUID, platform, chatUserID string) (*DigitalTwin, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT platform, chat_user_id,
		       first_name, last_name, city, country,
		       company, job_title, bio, status_text,
		       followers_count, pinterest_url, pinterest_boards,
		       facebook_name, message_count, lead_score
		FROM digital_twins
		WHERE workspace_id=$1 AND platform=$2 AND chat_user_id=$3`,
		workspaceID, platform, chatUserID,
	)

	var t DigitalTwin
	var boardsJSON []byte
	var firstName, lastName, city, country sql.NullString
	var company, jobTitle, bio, status sql.NullString
	var pinterestURL, facebookName sql.NullString

	err := row.Scan(
		&t.Platform, &t.ChatUserID,
		&firstName, &lastName, &city, &country,
		&company, &jobTitle, &bio, &status,
		&t.FollowersCount, &pinterestURL, &boardsJSON,
		&facebookName, &t.MessageCount, &t.LeadScore,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	t.FirstName = firstName.String
	t.LastName = lastName.String
	t.City = city.String
	t.Country = country.String
	t.Company = company.String
	t.JobTitle = jobTitle.String
	t.Bio = bio.String
	t.Status = status.String
	t.PinterestURL = pinterestURL.String
	t.FacebookName = facebookName.String

	if len(boardsJSON) > 2 { // more than "[]"
		_ = json.Unmarshal(boardsJSON, &t.PinterestBoards)
	}
	return &t, nil
}
