package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
	vkapi "github.com/dinarasaurae/inbetwin-social-service/internal/vk"
)

var topicWordPattern = regexp.MustCompile(`[\p{L}]{3,}`)

var topicStopwords = map[string]struct{}{
	"это": {}, "для": {}, "как": {}, "что": {}, "или": {}, "под": {}, "при": {}, "без": {}, "про": {},
	"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "your": {}, "have": {}, "will": {},
	"есть": {}, "если": {}, "когда": {}, "чтобы": {}, "после": {}, "перед": {}, "можно": {}, "вам": {},
	"нас": {}, "мы": {}, "они": {}, "она": {}, "оно": {}, "так": {}, "всё": {}, "все": {},
}

func (s *VKService) GetDiscoveryStatus(ctx context.Context, userID uuid.UUID) (*models.VKDiscoveryStatus, error) {
	platform, scope, token, _, err := s.loadUserConnectionMeta(ctx, userID)
	if err != nil {
		return nil, err
	}

	client := vkapi.NewUserClient(token)

	adminCap := models.VKMethodCapability{Method: "groups.get(filter=admin)", Supported: true}
	_, err = client.GroupsGetAdmin(ctx)
	if err != nil {
		adminCap = capabilityFromError(adminCap.Method, err)
	}

	subsCap := models.VKMethodCapability{Method: "users.getSubscriptions", Supported: true}
	subsResp, err := client.UsersGetSubscriptions(ctx, 100)
	if err != nil {
		subsCap = capabilityFromError(subsCap.Method, err)
	}

	canInfer := adminCap.Supported
	if !canInfer && subsCap.Supported {
		canInfer = subscriptionsContainAdminSignals(subsResp)
	}

	return &models.VKDiscoveryStatus{
		TokenPlatform:                        platform,
		TokenScope:                           scope,
		AdminGroups:                          adminCap,
		Subscriptions:                        subsCap,
		CanInferAdminGroupsFromSubscriptions: canInfer,
	}, nil
}

func (s *VKService) GetUserDataSnapshot(ctx context.Context, userID uuid.UUID) (*models.VKUserDataSnapshot, error) {
	platform, scope, _, _, err := s.loadUserConnectionMeta(ctx, userID)
	if err != nil {
		return nil, err
	}

	notes := []string{}
	snapshot := &models.VKUserDataSnapshot{
		TokenPlatform: platform,
		TokenScope:    scope,
		Posts:         models.VKMethodCapability{Method: "wall.get", Supported: true},
		Subscriptions: models.VKMethodCapability{Method: "users.getSubscriptions", Supported: true},
	}

	profile, err := s.GetUserProfile(ctx, userID)
	if err == nil {
		snapshot.Profile = profile
	} else {
		notes = append(notes, "Профиль VK сейчас не удалось загрузить этим токеном.")
	}

	posts, err := s.GetUserPosts(ctx, userID, 20)
	if err != nil {
		snapshot.Posts = capabilityFromError("wall.get", err)
	} else {
		texts := make([]string, 0, len(posts.Items))
		for _, item := range posts.Items {
			if strings.TrimSpace(item.Text) != "" {
				texts = append(texts, item.Text)
			}
		}
		snapshot.PostsCount = posts.Count
		snapshot.PostTopics = extractTopTopics(texts, 6)
	}

	subs, err := s.GetUserSubscriptions(ctx, userID)
	if err != nil {
		snapshot.Subscriptions = capabilityFromError("users.getSubscriptions", err)
		notes = append(notes, "Подписки пользователя VK сейчас не отдаёт этому токену.")
	} else {
		snapshot.SubscriptionsSummary = buildSubscriptionSummary(subs)
	}
	snapshot.Notes = notes
	return snapshot, nil
}

func (s *VKService) BootstrapContext(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID, count int, includeSubscriptions bool) (*models.VKContextBootstrapData, error) {
	imported, err := s.SyncPosts(ctx, userID, integrationID, count)
	if err != nil {
		return nil, err
	}

	workspace, err := s.GetWorkspace(ctx, userID, integrationID)
	if err != nil {
		return nil, err
	}
	if workspace.Integration == nil {
		return nil, fmt.Errorf("not_found: integration not found")
	}

	var owner *models.VKUserProfile
	owner, _ = s.GetUserProfile(ctx, userID)

	var subsSummary *models.VKSubscriptionSummary
	var subsNote string
	if includeSubscriptions {
		if subs, err := s.GetUserSubscriptions(ctx, userID); err == nil {
			subsSummary = buildSubscriptionSummary(subs)
		} else {
			subsNote = "VK не дал подписки пользователя текущему токену."
		}
	}

	setupWarnings := buildSetupWarnings(workspace.BusinessSnapshot)

	return &models.VKContextBootstrapData{
		IntegrationID:        integrationID.String(),
		GroupID:              workspace.Integration.GroupID,
		GroupName:            workspace.Integration.GroupName,
		GroupScreenName:      workspace.Integration.GroupScreenName,
		ImportedPosts:        imported,
		TotalPosts:           workspace.Integration.PostsCount,
		MessageCount:         workspace.Integration.MessageCount,
		LeadCount:            workspace.Integration.LeadCount,
		ContextReady:         workspace.Integration.ContextReady,
		TopTopics:            workspace.BusinessSnapshot.ContentSignals,
		OwnerProfile:         owner,
		SubscriptionsSummary: subsSummary,
		SubscriptionsNote:    subsNote,
		BusinessSnapshot:     workspace.BusinessSnapshot,
		SetupWarnings:        setupWarnings,
	}, nil
}

func (s *VKService) GetWorkspace(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID) (*models.VKWorkspaceData, error) {
	integ, _, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return nil, err
	}

	integ.PostsCount, integ.MessageCount, integ.LeadCount, _ = s.integrationCounts(ctx, integ.ID)
	integ.ContextReady = integ.PostsCount > 0 || integ.MessageCount > 0

	settings, err := s.GetAgentSettings(ctx, userID, integ.ID)
	if err != nil {
		return nil, err
	}

	posts, err := s.fetchRecentPosts(ctx, integ.ID, 6)
	if err != nil {
		return nil, err
	}
	messages, err := s.fetchRecentMessages(ctx, integ.ID, 20)
	if err != nil {
		return nil, err
	}
	leads, err := s.fetchRecentLeads(ctx, integ.ID, 10)
	if err != nil {
		return nil, err
	}
	drafts, err := s.ListDrafts(ctx, userID, integ.ID, 8)
	if err != nil {
		return nil, err
	}
	analytics, err := s.computeWorkspaceAnalytics(ctx, integ.ID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.buildBusinessSnapshot(ctx, userID, integ, posts, leads, messages, settings)
	if err != nil {
		return nil, err
	}
	recommendations := buildWorkspaceRecommendations(snapshot, analytics, drafts)

	return &models.VKWorkspaceData{
		Integration:      integ,
		BusinessSnapshot: snapshot,
		RecentPosts:      posts,
		RecentMessages:   messages,
		Leads:            leads,
		Drafts:           drafts,
		Recommendations:  recommendations,
		Analytics:        analytics,
		AgentSettings:    settings,
	}, nil
}

func (s *VKService) loadUserConnectionMeta(ctx context.Context, userID uuid.UUID) (platform, scope, token string, vkUserID int64, err error) {
	var enc, iv []byte
	err = s.db.QueryRowContext(ctx,
		`SELECT vk_user_id, platform, access_token_enc, access_token_iv, scope
		   FROM vk_user_connections
		  WHERE user_id=$1`,
		userID,
	).Scan(&vkUserID, &platform, &enc, &iv, &scope)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("not_found: VK user connection not found")
	}
	token, err = s.decryptToken(enc, iv)
	return platform, scope, token, vkUserID, err
}

func (s *VKService) integrationCounts(ctx context.Context, integrationID uuid.UUID) (posts int, messages int, leads int, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM vk_posts WHERE integration_id=$1),
			(SELECT COUNT(*) FROM vk_messages WHERE integration_id=$1),
			(SELECT COUNT(*) FROM vk_lead_profiles WHERE integration_id=$1)
	`, integrationID).Scan(&posts, &messages, &leads)
	return
}

func (s *VKService) fetchRecentPosts(ctx context.Context, integrationID uuid.UUID, limit int) ([]models.VKWorkspacePost, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, vk_post_id, COALESCE(text,''), has_media, COALESCE(media_type,''), COALESCE(likes_count,0), COALESCE(comments_count,0), posted_at
		  FROM vk_posts
		 WHERE integration_id=$1
		 ORDER BY posted_at DESC
		 LIMIT $2`,
		integrationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := []models.VKWorkspacePost{}
	for rows.Next() {
		var p models.VKWorkspacePost
		if err := rows.Scan(&p.ID, &p.VKPostID, &p.Text, &p.HasMedia, &p.MediaType, &p.LikesCount, &p.CommentsCount, &p.PostedAt); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func (s *VKService) fetchRecentMessages(ctx context.Context, integrationID uuid.UUID, limit int) ([]models.VKWorkspaceMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			m.id, m.from_vk_user_id, COALESCE(m.text,''), m.is_incoming, m.is_processed, m.received_at,
			d.id, d.intent, d.confidence, d.safe_intent, d.status, d.source, d.draft_text, d.rationale,
			COALESCE(d.knowledge_snippets, '[]'::jsonb)
		  FROM vk_messages m
		  LEFT JOIN vk_reply_drafts d ON d.inbound_message_id = m.id
		 WHERE m.integration_id=$1
		 ORDER BY m.received_at DESC
		 LIMIT $2`,
		integrationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.VKWorkspaceMessage{}
	for rows.Next() {
		var item models.VKWorkspaceMessage
		var draftID sql.NullString
		var intent, status, source, draftText, rationale string
		var conf float64
		var safe bool
		var snippetsRaw []byte
		if err := rows.Scan(
			&item.ID, &item.FromVKUserID, &item.Text, &item.IsIncoming, &item.IsProcessed, &item.ReceivedAt,
			&draftID, &intent, &conf, &safe, &status, &source, &draftText, &rationale, &snippetsRaw,
		); err != nil {
			return nil, err
		}
		if draftID.Valid {
			did, _ := uuid.Parse(draftID.String)
			var snippets []string
			_ = jsonUnmarshal(snippetsRaw, &snippets)
			item.Draft = &models.VKReplyDraft{
				ID:                did,
				IntegrationID:     integrationID,
				InboundMessageID:  item.ID,
				FromVKUserID:      item.FromVKUserID,
				Intent:            intent,
				Confidence:        conf,
				SafeIntent:        safe,
				Status:            models.VKDraftStatus(status),
				Source:            source,
				DraftText:         draftText,
				Rationale:         rationale,
				KnowledgeSnippets: snippets,
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *VKService) fetchRecentLeads(ctx context.Context, integrationID uuid.UUID, limit int) ([]models.VKWorkspaceLead, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, vk_user_id, COALESCE(first_name,''), COALESCE(last_name,''), COALESCE(city,''), COALESCE(country,''), COALESCE(about,''), COALESCE(status,''), COALESCE(followers_count,0), last_enriched_at
		  FROM vk_lead_profiles
		 WHERE integration_id=$1
		 ORDER BY COALESCE(last_enriched_at, created_at) DESC
		 LIMIT $2`,
		integrationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.VKWorkspaceLead{}
	for rows.Next() {
		var lead models.VKWorkspaceLead
		if err := rows.Scan(&lead.ID, &lead.VKUserID, &lead.FirstName, &lead.LastName, &lead.City, &lead.Country, &lead.About, &lead.Status, &lead.FollowersCount, &lead.LastEnrichedAt); err != nil {
			return nil, err
		}
		out = append(out, lead)
	}
	return out, rows.Err()
}

func (s *VKService) buildBusinessSnapshot(ctx context.Context, userID uuid.UUID, integ *models.VKIntegration, posts []models.VKWorkspacePost, leads []models.VKWorkspaceLead, messages []models.VKWorkspaceMessage, settings *models.VKAgentSettings) (*models.VKBusinessSnapshot, error) {
	postTexts := make([]string, 0, len(posts))
	for _, post := range posts {
		if strings.TrimSpace(post.Text) != "" {
			postTexts = append(postTexts, post.Text)
		}
	}
	topTopics := extractTopTopics(postTexts, 6)
	longPollEnabled, longPollWarning := s.longPollEnabled(ctx, userID, integ.ID)

	stage := "Foundation"
	if integ.MessageCount >= 5 {
		stage = "Conversation"
	}
	if integ.MessageCount >= 10 && integ.LeadCount >= 5 {
		stage = "Operational"
	}

	offerSignals := append([]string{}, topTopics...)
	if len(offerSignals) > 4 {
		offerSignals = offerSignals[:4]
	}

	knowledgeSignals := []string{}
	if settings != nil && settings.RAGEnabled {
		knowledgeSignals = append(knowledgeSignals, "RAG-контекст можно подключать к ответам")
	}
	if settings != nil && settings.ToneOfVoice != "" {
		knowledgeSignals = append(knowledgeSignals, "Настроен tone of voice для агента")
	}

	missingSignals := []string{}
	if integ.PostsCount == 0 {
		missingSignals = append(missingSignals, "Нет импортированных постов")
	}
	if integ.MessageCount == 0 {
		missingSignals = append(missingSignals, "Нет входящих сообщений для обучения агента")
	}
	if !longPollEnabled {
		missingSignals = append(missingSignals, "Long Poll API выключен")
	}

	recommended := []string{}
	if integ.PostsCount < 10 {
		recommended = append(recommended, "Импортировать больше постов для richer business summary")
	}
	if !longPollEnabled {
		recommended = append(recommended, longPollWarning)
	}
	if integ.MessageCount == 0 {
		recommended = append(recommended, "Получить первые входящие сообщения и включить draft-first")
	}
	if settings != nil && !settings.AutoReplyEnabled {
		recommended = append(recommended, "Оставить auto-reply выключенным до накопления FAQ и истории диалогов")
	}

	positioning := "VK-сообщество с фокусом на лидогенерацию и первичную квалификацию"
	if len(topTopics) > 0 {
		positioning = fmt.Sprintf("Ключевые темы сообщества: %s", strings.Join(topTopics[:minInt(len(topTopics), 3)], ", "))
	}

	audienceSummary := "Пока мало сигналов об аудитории."
	if len(leads) > 0 {
		audienceSummary = fmt.Sprintf("Уже накоплено %d лидов из VK диалогов.", integ.LeadCount)
	}

	summary := fmt.Sprintf("%s (@%s) подключено. Постов: %d, сообщений: %d, лидов: %d.",
		integ.GroupName, integ.GroupScreenName, integ.PostsCount, integ.MessageCount, integ.LeadCount,
	)

	return &models.VKBusinessSnapshot{
		Summary:               summary,
		TwinStage:             stage,
		Positioning:           positioning,
		AudienceSummary:       audienceSummary,
		OfferSignals:          offerSignals,
		ContentSignals:        topTopics,
		KnowledgeSignals:      knowledgeSignals,
		MissingSignals:        missingSignals,
		RecommendedActions:    recommended,
		LongPollEnabled:       longPollEnabled,
		ReadyForConversations: longPollEnabled && integ.PostsCount > 0,
	}, nil
}

func (s *VKService) longPollEnabled(ctx context.Context, userID, integrationID uuid.UUID) (bool, string) {
	integ, token, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return false, "Не удалось проверить Long Poll API."
	}
	client := vkapi.NewClient(token, integ.GroupID)
	if _, err := client.GroupsGetLongPollServer(ctx, integ.GroupID); err != nil {
		if strings.Contains(err.Error(), "longpoll for this group is not enabled") {
			return false, "Включите Сообщения сообщества и Long Poll API в настройках группы VK."
		}
		return false, "Long Poll API сейчас недоступен."
	}
	return true, ""
}

func buildSubscriptionSummary(resp *vkapi.SubscriptionsExtendedResponse) *models.VKSubscriptionSummary {
	if resp == nil {
		return nil
	}
	summary := &models.VKSubscriptionSummary{
		Count:       resp.Count,
		SampleNames: []string{},
	}
	for _, item := range resp.Items {
		switch item.Type {
		case "group", "page", "event":
			summary.GroupsCount++
			if item.Name != "" && len(summary.SampleNames) < 5 {
				summary.SampleNames = append(summary.SampleNames, item.Name)
			}
		default:
			summary.ProfilesCount++
			fullName := strings.TrimSpace(item.FirstName + " " + item.LastName)
			if fullName != "" && len(summary.SampleNames) < 5 {
				summary.SampleNames = append(summary.SampleNames, fullName)
			}
		}
	}
	return summary
}

func capabilityFromError(method string, err error) models.VKMethodCapability {
	cap := models.VKMethodCapability{Method: method, Supported: false, ErrorMessage: err.Error()}
	var apiErr *vkapi.APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		cap.ErrorCode = apiErr.Code
		cap.ErrorMessage = apiErr.Message
	}
	if strings.Contains(err.Error(), "vk error 1051") {
		cap.ErrorCode = 1051
	}
	return cap
}

func subscriptionsContainAdminSignals(resp *vkapi.SubscriptionsExtendedResponse) bool {
	if resp == nil {
		return false
	}
	for _, item := range resp.Items {
		if item.AdminLevel > 0 || item.IsAdmin > 0 {
			return true
		}
	}
	return false
}

func extractTopTopics(texts []string, limit int) []string {
	counts := map[string]int{}
	for _, text := range texts {
		words := topicWordPattern.FindAllString(strings.ToLower(text), -1)
		for _, word := range words {
			if _, skip := topicStopwords[word]; skip {
				continue
			}
			counts[word]++
		}
	}
	type kv struct {
		Key string
		Val int
	}
	items := make([]kv, 0, len(counts))
	for k, v := range counts {
		items = append(items, kv{k, v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Val == items[j].Val {
			return items[i].Key < items[j].Key
		}
		return items[i].Val > items[j].Val
	})
	result := []string{}
	for _, item := range items {
		result = append(result, item.Key)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func buildSetupWarnings(snapshot *models.VKBusinessSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	warnings := append([]string{}, snapshot.MissingSignals...)
	if !snapshot.LongPollEnabled {
		warnings = append(warnings, "В группе не включён Long Poll API, поэтому real-time сообщения не синхронизируются.")
	}
	return warnings
}

func buildWorkspaceRecommendations(snapshot *models.VKBusinessSnapshot, analytics models.VKWorkspaceAnalytics, drafts []models.VKReplyDraft) []string {
	recs := []string{}
	if snapshot != nil {
		recs = append(recs, snapshot.RecommendedActions...)
	}
	if analytics.PendingDrafts > 0 {
		recs = append(recs, fmt.Sprintf("Проверьте %d черновиков в draft-first очереди.", analytics.PendingDrafts))
	}
	if analytics.FirstResponseTimeMinutes > 30 {
		recs = append(recs, "Снизьте first response time: включите draft-first и подготовьте FAQ-ответы.")
	}
	if len(drafts) == 0 {
		recs = append(recs, "После первых входящих здесь появятся рекомендованные ответы и handoff-метрики.")
	}
	return uniqueStrings(recs)
}

func (s *VKService) computeWorkspaceAnalytics(ctx context.Context, integrationID uuid.UUID) (models.VKWorkspaceAnalytics, error) {
	analytics := models.VKWorkspaceAnalytics{}
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM vk_messages WHERE integration_id=$1 AND is_incoming=TRUE),
			(SELECT COUNT(*) FROM vk_messages WHERE integration_id=$1 AND is_incoming=FALSE),
			(SELECT COUNT(*) FROM vk_reply_drafts WHERE integration_id=$1 AND status='pending'),
			(SELECT COUNT(*) FROM vk_lead_profiles WHERE integration_id=$1)
	`, integrationID).Scan(&analytics.IncomingMessages, &analytics.OutgoingMessages, &analytics.PendingDrafts, &analytics.Leads); err != nil {
		return analytics, err
	}

	rows, err := s.db.QueryContext(ctx, `
		WITH inbound AS (
			SELECT id, from_vk_user_id, received_at
			  FROM vk_messages
			 WHERE integration_id=$1 AND is_incoming=TRUE
		)
		SELECT EXTRACT(EPOCH FROM AVG(o.received_at - i.received_at))/60.0
		  FROM inbound i
		  JOIN LATERAL (
			 SELECT received_at
			   FROM vk_messages
			  WHERE integration_id=$1
			    AND is_incoming=FALSE
			    AND from_vk_user_id=i.from_vk_user_id
			    AND received_at >= i.received_at
			  ORDER BY received_at ASC
			  LIMIT 1
		  ) o ON TRUE
	`, integrationID)
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			var avg sql.NullFloat64
			if err := rows.Scan(&avg); err == nil && avg.Valid {
				analytics.FirstResponseTimeMinutes = math.Round(avg.Float64*10) / 10
			}
		}
	}

	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(AVG(CASE WHEN status IN ('pending','approved','sent') THEN 1.0 ELSE 0.0 END), 0),
			COALESCE(
				(SELECT COUNT(DISTINCT from_vk_user_id)::float
				   FROM vk_messages
				  WHERE integration_id=$1 AND is_incoming=FALSE) /
				NULLIF((SELECT COUNT(DISTINCT from_vk_user_id)::float
				          FROM vk_messages
				         WHERE integration_id=$1 AND is_incoming=TRUE), 0),
				0
			)
		  FROM vk_reply_drafts
		 WHERE integration_id=$1
	`, integrationID).Scan(&analytics.HandoffRate, &analytics.LeadConversionRate); err != nil {
		return analytics, err
	}

	confRows, err := s.db.QueryContext(ctx, `
		SELECT intent, COALESCE(AVG(confidence),0), COUNT(*)
		  FROM vk_reply_drafts
		 WHERE integration_id=$1
		 GROUP BY intent
		 ORDER BY COUNT(*) DESC, intent ASC
	`, integrationID)
	if err != nil {
		return analytics, err
	}
	defer confRows.Close()
	for confRows.Next() {
		var item models.VKIntentConfidence
		if err := confRows.Scan(&item.Intent, &item.AvgConfidence, &item.Count); err != nil {
			return analytics, err
		}
		item.AvgConfidence = math.Round(item.AvgConfidence*100) / 100
		analytics.ConfidenceByIntent = append(analytics.ConfidenceByIntent, item)
	}
	return analytics, confRows.Err()
}

func uniqueStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func jsonUnmarshal(data []byte, dest interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, dest)
}
