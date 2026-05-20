package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	openai "github.com/sashabaranov/go-openai"

	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
)

type ragSearchResult struct {
	Text string `json:"text"`
}

type ragSearchResponse struct {
	Results []ragSearchResult `json:"results"`
}

type intentDecision struct {
	Intent     string
	Confidence float64
	Safe       bool
	Rationale  string
}

func (s *VKService) GetAgentSettings(ctx context.Context, userID, integrationID uuid.UUID) (*models.VKAgentSettings, error) {
	if _, _, err := s.loadIntegration(ctx, userID, integrationID); err != nil {
		return nil, err
	}
	return s.ensureAgentSettings(ctx, integrationID)
}

func (s *VKService) UpdateAgentSettings(ctx context.Context, userID uuid.UUID, req models.VKAgentSettingsUpdateRequest) (*models.VKAgentSettings, error) {
	integID, err := uuid.Parse(req.IntegrationID)
	if err != nil {
		return nil, fmt.Errorf("invalid_request: integration_id is invalid")
	}
	if _, _, err := s.loadIntegration(ctx, userID, integID); err != nil {
		return nil, err
	}

	current, err := s.ensureAgentSettings(ctx, integID)
	if err != nil {
		return nil, err
	}

	draftFirst := current.DraftFirst
	if req.DraftFirst != nil {
		draftFirst = *req.DraftFirst
	}
	autoReplyEnabled := current.AutoReplyEnabled
	if req.AutoReplyEnabled != nil {
		autoReplyEnabled = *req.AutoReplyEnabled
	}
	safeIntents := current.SafeIntents
	if req.SafeIntents != nil {
		safeIntents = *req.SafeIntents
	}
	if len(safeIntents) == 0 {
		safeIntents = []string{"faq", "hours", "basic_prices", "qualification"}
	}
	toneOfVoice := current.ToneOfVoice
	if req.ToneOfVoice != nil {
		toneOfVoice = *req.ToneOfVoice
	}
	forbiddenPromises := current.ForbiddenPromises
	if req.ForbiddenPromises != nil {
		forbiddenPromises = *req.ForbiddenPromises
	}
	escalationPolicy := current.EscalationPolicy
	if req.EscalationPolicy != nil {
		escalationPolicy = *req.EscalationPolicy
	}
	ragEnabled := current.RAGEnabled
	if req.RAGEnabled != nil {
		ragEnabled = *req.RAGEnabled
	}

	settings := &models.VKAgentSettings{}
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO vk_agent_settings
			(integration_id, draft_first, auto_reply_enabled, safe_intents, tone_of_voice, forbidden_promises, escalation_policy, rag_enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (integration_id) DO UPDATE SET
			draft_first=EXCLUDED.draft_first,
			auto_reply_enabled=EXCLUDED.auto_reply_enabled,
			safe_intents=EXCLUDED.safe_intents,
			tone_of_voice=EXCLUDED.tone_of_voice,
			forbidden_promises=EXCLUDED.forbidden_promises,
			escalation_policy=EXCLUDED.escalation_policy,
			rag_enabled=EXCLUDED.rag_enabled,
			updated_at=NOW()
		RETURNING id, integration_id, draft_first, auto_reply_enabled, safe_intents, tone_of_voice, forbidden_promises, escalation_policy, rag_enabled, orchestration_mode, created_at, updated_at
	`, integID, draftFirst, autoReplyEnabled, pq.Array(safeIntents), toneOfVoice, pq.Array(forbiddenPromises), escalationPolicy, ragEnabled).
		Scan(&settings.ID, &settings.IntegrationID, &settings.DraftFirst, &settings.AutoReplyEnabled, pq.Array(&settings.SafeIntents), &settings.ToneOfVoice, pq.Array(&settings.ForbiddenPromises), &settings.EscalationPolicy, &settings.RAGEnabled, &settings.OrchestrationMode, &settings.CreatedAt, &settings.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *VKService) ListDrafts(ctx context.Context, userID, integrationID uuid.UUID, limit int) ([]models.VKReplyDraft, error) {
	if _, _, err := s.loadIntegration(ctx, userID, integrationID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, integration_id, inbound_message_id, from_vk_user_id, intent, confidence, safe_intent, status, source, draft_text, rationale, knowledge_snippets, sent_message_id, approved_by, generated_at, approved_at, sent_at,
		       orchestration_source, llm_latency_ms, fallback_reason, prompt_tokens, completion_tokens, used_agent_id, used_tools
		  FROM vk_reply_drafts
		 WHERE integration_id=$1
		 ORDER BY generated_at DESC
		 LIMIT $2`,
		integrationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDraftRows(rows)
}

func (s *VKService) GenerateDraft(ctx context.Context, userID, integrationID, messageID uuid.UUID, force bool) (*models.VKReplyDraft, error) {
	_, _, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return nil, err
	}
	return s.generateDraftForMessage(ctx, userID, integrationID, messageID, force, true, "legacy", nil)
}

func (s *VKService) ApproveAndSendDraft(ctx context.Context, userID, draftID uuid.UUID, textOverride string) (*models.VKReplyDraft, error) {
	draft, integ, err := s.loadDraft(ctx, userID, draftID)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(textOverride)
	if text == "" {
		text = draft.DraftText
	}
	sentID, err := s.sendMessageInternal(ctx, userID, integ.ID, draft.FromVKUserID, draft.PeerID, text)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE vk_reply_drafts
		   SET status=$1, draft_text=$2, approved_by=$3, approved_at=$4, sent_at=$4, sent_message_id=$5
		 WHERE id=$6`,
		models.VKDraftStatusSent, text, userID, now, sentID, draft.ID,
	); err != nil {
		return nil, err
	}
	draft.Status = models.VKDraftStatusSent
	draft.DraftText = text
	draft.ApprovedBy = &userID
	draft.ApprovedAt = &now
	draft.SentAt = &now
	draft.SentMessageID = &sentID
	return draft, nil
}

func (s *VKService) ensureAgentSettings(ctx context.Context, integrationID uuid.UUID) (*models.VKAgentSettings, error) {
	settings := &models.VKAgentSettings{}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO vk_agent_settings (integration_id, orchestration_mode)
		VALUES ($1, '')
		ON CONFLICT (integration_id) DO UPDATE SET integration_id=EXCLUDED.integration_id
		RETURNING id, integration_id, draft_first, auto_reply_enabled, safe_intents, tone_of_voice, forbidden_promises, escalation_policy, rag_enabled, orchestration_mode, created_at, updated_at
	`, integrationID).Scan(&settings.ID, &settings.IntegrationID, &settings.DraftFirst, &settings.AutoReplyEnabled, pq.Array(&settings.SafeIntents), &settings.ToneOfVoice, pq.Array(&settings.ForbiddenPromises), &settings.EscalationPolicy, &settings.RAGEnabled, &settings.OrchestrationMode, &settings.CreatedAt, &settings.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// effectiveOrchestrationMode returns the orchestration mode to use for a given
// integration. The per-integration OrchestrationMode field (from vk_agent_settings)
// takes precedence over the service-wide LLM_ORCHESTRATION_MODE env var.
//
// Priority: settings.OrchestrationMode (non-empty) → s.cfg.LLMOrchestrationMode → "legacy"
func (s *VKService) effectiveOrchestrationMode(settings *models.VKAgentSettings) string {
	if settings != nil && settings.OrchestrationMode != "" {
		return settings.OrchestrationMode
	}
	if s.cfg.LLMOrchestrationMode != "" {
		return s.cfg.LLMOrchestrationMode
	}
	return "legacy"
}

// processInboundMessage loads per-integration settings once, resolves the
// effective orchestration mode (per-integration override takes precedence over
// the service-level LLM_ORCHESTRATION_MODE), then dispatches.
func (s *VKService) processInboundMessage(ctx context.Context, userID, integrationID, messageID uuid.UUID) {
	settings, err := s.ensureAgentSettings(ctx, integrationID)
	if err != nil {
		log.Printf("[vk-agent] settings: %v", err)
		return
	}

	// Notify the workspace owner about the new incoming message.
	go s.pushClient.SendToUser(ctx, userID, InternalPushRequest{
		Type:  "NEW_MESSAGE",
		Title: "Новое сообщение",
		Body:  "Клиент написал вам в VK",
	})

	switch s.effectiveOrchestrationMode(settings) {
	case "llm_service":
		s.processInboundMessageLLM(ctx, userID, integrationID, messageID, false, settings)
	case "hybrid":
		s.processInboundMessageLLM(ctx, userID, integrationID, messageID, true, settings)
	default: // "legacy"
		s.processInboundMessageLegacy(ctx, userID, integrationID, messageID, settings)
	}
}

// processInboundMessageLegacy is the original, unchanged draft-generation
// pipeline using the vk_agent built-in logic. settings is pre-loaded by the
// dispatcher to avoid a redundant DB call.
func (s *VKService) processInboundMessageLegacy(ctx context.Context, userID, integrationID, messageID uuid.UUID, settings *models.VKAgentSettings) {
	if !settings.DraftFirst && !settings.AutoReplyEnabled {
		_, _ = s.db.ExecContext(ctx, `UPDATE vk_messages SET is_processed=TRUE WHERE id=$1`, messageID)
		return
	}
	if _, err := s.generateDraftForMessage(ctx, userID, integrationID, messageID, false, true, "legacy", nil); err != nil {
		log.Printf("[vk-agent][legacy] generate draft: %v", err)
	}
}

// processInboundMessageLLM routes through the llm-service orchestrator.
// withFallback=true means hybrid mode: on error fall back to legacy.
// settings is pre-loaded by the dispatcher.
func (s *VKService) processInboundMessageLLM(ctx context.Context, userID, integrationID, messageID uuid.UUID, withFallback bool, settings *models.VKAgentSettings) {
	if s.llmClient == nil {
		log.Printf("[vk-agent][llm] llm client not configured, falling back to legacy")
		s.processInboundMessageLegacy(ctx, userID, integrationID, messageID, settings)
		return
	}

	if !settings.DraftFirst && !settings.AutoReplyEnabled {
		_, _ = s.db.ExecContext(ctx, `UPDATE vk_messages SET is_processed=TRUE WHERE id=$1`, messageID)
		return
	}

	inbound, err := s.loadInboundMessage(ctx, userID, integrationID, messageID)
	if err != nil {
		log.Printf("[vk-agent][llm] load message: %v", err)
		return
	}

	workspace, err := s.GetWorkspace(ctx, userID, integrationID)
	if err != nil {
		log.Printf("[vk-agent][llm] workspace: %v", err)
	}

	// Build business snapshot summary for context.
	snapshotSummary := ""
	if workspace != nil && workspace.BusinessSnapshot != nil {
		snapshotSummary = workspace.BusinessSnapshot.Summary
	}

	// Fetch the lead's digital twin (name, city, occupation, Pinterest interests…).
	// Non-blocking: if the twin service isn't wired or the profile not yet enriched,
	// twin will be nil and the LLM falls back to anonymous mode.
	chatUserID := fmt.Sprintf("%d", inbound.FromVKUserID)
	var twin *DigitalTwin
	if s.twinSvc != nil {
		twin = s.twinSvc.GetOrBuild(ctx, userID, "vk", chatUserID)
	}

	req := VKProcessRequest{
		IntegrationID: integrationID.String(),
		ChatUserID:    chatUserID,
		Platform:      "vk",
		Message:       inbound.Text,
		Context: &VKProcessContext{
			ToneOfVoice:       settings.ToneOfVoice,
			SafeIntents:       settings.SafeIntents,
			ForbiddenPromises: settings.ForbiddenPromises,
			EscalationPolicy:  settings.EscalationPolicy,
			AutoReplyEnabled:  settings.AutoReplyEnabled,
			BusinessSnapshot:  snapshotSummary,
			DigitalTwin:       twin,
		},
	}

	t0 := time.Now()
	decision, err := s.llmClient.ProcessVKMessage(ctx, userID, req)
	latencyMs := int(time.Since(t0).Milliseconds())

	if err != nil {
		fallbackReason := err.Error()
		log.Printf("[vk-agent][llm] orchestration error (latency=%dms): %v", latencyMs, err)
		if withFallback {
			log.Printf("[vk-agent][hybrid] falling back to legacy for message %s", messageID)
			if _, fbErr := s.generateDraftForMessage(ctx, userID, integrationID, messageID, true, true, "fallback_legacy", &fallbackReason); fbErr != nil {
				log.Printf("[vk-agent][hybrid] fallback generate draft: %v", fbErr)
			}
		}
		return
	}

	// Persist the decision as a draft using existing infrastructure.
	obs := &draftObservability{
		source:           "llm_service",
		latencyMs:        &latencyMs,
		promptTokens:     &decision.PromptTokens,
		completionTokens: &decision.CompletionTokens,
		usedAgentID:      decision.AgentID,
		usedTools:        decision.UsedTools,
	}

	draft := &models.VKReplyDraft{
		IntegrationID:     integrationID,
		InboundMessageID:  inbound.ID,
		FromVKUserID:      inbound.FromVKUserID,
		PeerID:            inbound.PeerID,
		Intent:            decision.Intent,
		Confidence:        decision.Confidence,
		SafeIntent:        decision.SafeIntent,
		Status:            models.VKDraftStatusPending,
		Source:            "llm_service",
		DraftText:         decision.DraftText,
		Rationale:         decision.Rationale,
		KnowledgeSnippets: decision.KnowledgeSnippets,
	}

	// Auto-send if orchestrator says auto_reply and policy allows.
	if decision.Mode == "auto_reply" && settings.AutoReplyEnabled && !settings.DraftFirst {
		sentID, sendErr := s.sendMessageInternal(ctx, userID, integrationID, inbound.FromVKUserID, inbound.PeerID, draft.DraftText)
		if sendErr != nil {
			draft.Status = models.VKDraftStatusFailed
			log.Printf("[vk-agent][llm] auto-send failed: %v", sendErr)
		} else {
			now := time.Now()
			draft.Status = models.VKDraftStatusAutoSent
			draft.SentAt = &now
			draft.SentMessageID = &sentID
		}
	}

	if err := s.persistDraftWithObs(ctx, draft, obs); err != nil {
		log.Printf("[vk-agent][llm] persist draft: %v", err)
		return
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE vk_messages SET is_processed=TRUE WHERE id=$1`, inbound.ID)
}

// draftObservability carries the optional metrics to be stored on the draft row.
type draftObservability struct {
	source           string
	latencyMs        *int
	fallbackReason   *string
	promptTokens     *int
	completionTokens *int
	usedAgentID      *uuid.UUID
	usedTools        []string
}

func (s *VKService) generateDraftForMessage(
	ctx context.Context,
	userID, integrationID, messageID uuid.UUID,
	force bool,
	allowAutoSend bool,
	orchSource string,
	fallbackReason *string,
) (*models.VKReplyDraft, error) {
	settings, err := s.ensureAgentSettings(ctx, integrationID)
	if err != nil {
		return nil, err
	}

	if !force {
		if existing, err := s.findDraftByMessage(ctx, messageID); err == nil && existing != nil {
			return existing, nil
		}
	}

	inbound, err := s.loadInboundMessage(ctx, userID, integrationID, messageID)
	if err != nil {
		return nil, err
	}

	workspace, err := s.GetWorkspace(ctx, userID, integrationID)
	if err != nil {
		return nil, err
	}
	intent := classifyIntent(inbound.Text)
	knowledge := []string{}
	if settings.RAGEnabled {
		knowledge = s.searchKnowledge(ctx, userID, inbound.Text)
	}
	draftText, source := s.composeDraft(ctx, workspace.BusinessSnapshot, settings, inbound.Text, intent, knowledge)
	if source == "knowledge" && intent.Intent == "faq" {
		intent.Confidence = maxFloat(intent.Confidence, 0.9)
		intent.Safe = true
		intent.Rationale = appendRationale(intent.Rationale, "Ответ найден в базе знаний.")
	}

	draft := &models.VKReplyDraft{
		IntegrationID:     integrationID,
		InboundMessageID:  inbound.ID,
		FromVKUserID:      inbound.FromVKUserID,
		PeerID:            inbound.PeerID,
		Intent:            intent.Intent,
		Confidence:        intent.Confidence,
		SafeIntent:        intent.Safe,
		Status:            models.VKDraftStatusPending,
		Source:            source,
		DraftText:         draftText,
		Rationale:         intent.Rationale,
		KnowledgeSnippets: knowledge,
	}

	if allowAutoSend && settings.AutoReplyEnabled && !settings.DraftFirst && isIntentAllowed(settings.SafeIntents, intent.Intent) && intent.Safe && intent.Confidence >= 0.84 {
		sentID, sendErr := s.sendMessageInternal(ctx, userID, integrationID, inbound.FromVKUserID, inbound.PeerID, draft.DraftText)
		if sendErr != nil {
			draft.Status = models.VKDraftStatusFailed
		} else {
			now := time.Now()
			draft.Status = models.VKDraftStatusAutoSent
			draft.SentAt = &now
			draft.SentMessageID = &sentID
		}
	}

	obs := &draftObservability{
		source:         orchSource,
		fallbackReason: fallbackReason,
	}
	if err := s.persistDraftWithObs(ctx, draft, obs); err != nil {
		return nil, err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE vk_messages SET is_processed=TRUE WHERE id=$1`, inbound.ID)
	return draft, nil
}

// persistDraftWithObs upserts the draft row including all observability columns.
func (s *VKService) persistDraftWithObs(ctx context.Context, draft *models.VKReplyDraft, obs *draftObservability) error {
	if obs == nil {
		obs = &draftObservability{source: "legacy"}
	}
	if obs.usedTools == nil {
		obs.usedTools = []string{}
	}
	knowledgeJSON, _ := json.Marshal(draft.KnowledgeSnippets)
	usedToolsJSON, _ := json.Marshal(obs.usedTools)

	return s.db.QueryRowContext(ctx, `
		INSERT INTO vk_reply_drafts
			(integration_id, inbound_message_id, from_vk_user_id,
			 intent, confidence, safe_intent, status, source, draft_text, rationale,
			 knowledge_snippets, sent_message_id, sent_at,
			 orchestration_source, llm_latency_ms, fallback_reason,
			 prompt_tokens, completion_tokens, used_agent_id, used_tools)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (inbound_message_id) DO UPDATE SET
			intent=EXCLUDED.intent,
			confidence=EXCLUDED.confidence,
			safe_intent=EXCLUDED.safe_intent,
			status=EXCLUDED.status,
			source=EXCLUDED.source,
			draft_text=EXCLUDED.draft_text,
			rationale=EXCLUDED.rationale,
			knowledge_snippets=EXCLUDED.knowledge_snippets,
			sent_message_id=EXCLUDED.sent_message_id,
			sent_at=EXCLUDED.sent_at,
			orchestration_source=EXCLUDED.orchestration_source,
			llm_latency_ms=EXCLUDED.llm_latency_ms,
			fallback_reason=EXCLUDED.fallback_reason,
			prompt_tokens=EXCLUDED.prompt_tokens,
			completion_tokens=EXCLUDED.completion_tokens,
			used_agent_id=EXCLUDED.used_agent_id,
			used_tools=EXCLUDED.used_tools
		RETURNING id, generated_at
	`,
		draft.IntegrationID, draft.InboundMessageID, draft.FromVKUserID,
		draft.Intent, draft.Confidence, draft.SafeIntent, draft.Status, draft.Source,
		draft.DraftText, draft.Rationale,
		knowledgeJSON, draft.SentMessageID, draft.SentAt,
		obs.source, obs.latencyMs, obs.fallbackReason,
		obs.promptTokens, obs.completionTokens, obs.usedAgentID, usedToolsJSON,
	).Scan(&draft.ID, &draft.GeneratedAt)
}

func (s *VKService) composeDraft(ctx context.Context, snapshot *models.VKBusinessSnapshot, settings *models.VKAgentSettings, userMessage string, decision intentDecision, knowledge []string) (string, string) {
	if text, ok := draftFromKnowledge(userMessage, knowledge); ok {
		return text, "knowledge"
	}
	if s.draftProvider == nil || s.cfg.LLMAPIKey == "" {
		return templateDraft(snapshot, settings, userMessage, decision), "template"
	}
	systemPrompt := buildSystemPrompt(snapshot, settings, decision, knowledge)
	resp, err := s.draftProvider.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: s.cfg.LLMModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userMessage},
		},
		Temperature: 0.3,
		MaxTokens:   220,
	})
	if err != nil || len(resp.Choices) == 0 {
		return templateDraft(snapshot, settings, userMessage, decision), "template"
	}
	text := strings.TrimSpace(resp.Choices[0].Message.Content)
	if text == "" {
		return templateDraft(snapshot, settings, userMessage, decision), "template"
	}
	return text, s.draftProvider.Name()
}

func draftFromKnowledge(userMessage string, knowledge []string) (string, bool) {
	if len(knowledge) == 0 {
		return "", false
	}

	query := normalizeKnowledgeText(userMessage)
	preferredKeys := preferredKnowledgeKeys(query)
	if len(preferredKeys) == 0 {
		return "", false
	}

	for _, snippet := range knowledge {
		fields := parseKnowledgeFields(snippet)
		if len(fields) == 0 {
			continue
		}

		product := firstKnowledgeField(fields, "продукт", "товар", "название", "name")
		if product != "" && !knowledgeProductMatches(query, product) {
			continue
		}

		value := firstKnowledgeField(fields, preferredKeys...)
		if value == "" {
			continue
		}
		if product != "" {
			return ensureFinalPunctuation(product + ": " + value), true
		}
		return ensureFinalPunctuation(value), true
	}
	return "", false
}

func preferredKnowledgeKeys(normalizedQuery string) []string {
	var keys []string
	if containsAny(normalizedQuery, "способ применения", "как применять", "как принимать", "применять", "принимать", "дозиров", "капсул") {
		keys = append(keys, "способ применения", "применение", "дозировка")
	}
	if containsAny(normalizedQuery, "срок годности", "годен", "годна", "хранить", "хранение") {
		keys = append(keys, "срок годности", "годен до", "хранение")
	}
	return keys
}

func parseKnowledgeFields(snippet string) map[string]string {
	fields := map[string]string{}
	for _, part := range strings.Split(snippet, ",") {
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		key = normalizeKnowledgeFieldKey(key)
		value = strings.Trim(strings.TrimSpace(value), ".;")
		if key != "" && value != "" {
			fields[key] = value
		}
	}
	return fields
}

func normalizeKnowledgeFieldKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "_", " ")
	return strings.Join(strings.Fields(key), " ")
}

func firstKnowledgeField(fields map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fields[normalizeKnowledgeFieldKey(key)]); value != "" {
			return value
		}
	}
	return ""
}

func knowledgeProductMatches(normalizedQuery, product string) bool {
	normalizedProduct := normalizeKnowledgeText(product)
	if normalizedProduct == "" {
		return true
	}
	if strings.Contains(normalizedQuery, normalizedProduct) {
		return true
	}
	for _, token := range strings.Fields(normalizedProduct) {
		if len([]rune(token)) >= 4 && strings.Contains(normalizedQuery, token) {
			return true
		}
	}
	return false
}

func normalizeKnowledgeText(value string) string {
	value = strings.ToLower(strings.ReplaceAll(value, "ё", "е"))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'а' && r <= 'я', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func ensureFinalPunctuation(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	switch text[len(text)-1] {
	case '.', '!', '?':
		return text
	default:
		return text + "."
	}
}

func appendRationale(existing, addition string) string {
	existing = strings.TrimSpace(existing)
	addition = strings.TrimSpace(addition)
	if existing == "" {
		return addition
	}
	if addition == "" {
		return existing
	}
	return existing + " " + addition
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func buildSystemPrompt(snapshot *models.VKBusinessSnapshot, settings *models.VKAgentSettings, decision intentDecision, knowledge []string) string {
	tone := "спокойный, полезный, вежливый"
	if settings != nil && strings.TrimSpace(settings.ToneOfVoice) != "" {
		tone = settings.ToneOfVoice
	}
	sb := strings.Builder{}
	sb.WriteString("Ты отвечаешь от лица VK-сообщества бизнеса.\n")
	sb.WriteString("Стиль ответа: " + tone + ".\n")
	if snapshot != nil {
		sb.WriteString("Business summary: " + snapshot.Summary + "\n")
		if snapshot.Positioning != "" {
			sb.WriteString("Positioning: " + snapshot.Positioning + "\n")
		}
		if len(snapshot.OfferSignals) > 0 {
			sb.WriteString("Offer signals: " + strings.Join(snapshot.OfferSignals, ", ") + "\n")
		}
	}
	sb.WriteString("Detected intent: " + decision.Intent + ".\n")
	if settings != nil && len(settings.ForbiddenPromises) > 0 {
		sb.WriteString("Нельзя обещать: " + strings.Join(settings.ForbiddenPromises, "; ") + ".\n")
	}
	if settings != nil && settings.EscalationPolicy != "" {
		sb.WriteString("Если не хватает фактов или вопрос рискованный, следуй правилу: " + settings.EscalationPolicy + ".\n")
	}
	if len(knowledge) > 0 {
		sb.WriteString("Knowledge base snippets:\n- " + strings.Join(knowledge, "\n- ") + "\n")
	}
	sb.WriteString("Ответь коротко, конкретно, без выдуманных деталей. Если данных не хватает, задай один уточняющий вопрос.\n")
	return sb.String()
}

func templateDraft(snapshot *models.VKBusinessSnapshot, settings *models.VKAgentSettings, userMessage string, decision intentDecision) string {
	switch decision.Intent {
	case "hours":
		return "Спасибо за сообщение. Уточню актуальные часы работы и сразу вернусь с точным ответом."
	case "basic_prices":
		return "Спасибо за вопрос. Подскажу по базовой стоимости и, если нужно, помогу подобрать подходящий вариант под ваш запрос."
	case "qualification":
		return "Спасибо. Чтобы сориентировать вас точнее, напишите, пожалуйста, какой результат хотите получить и в какие сроки."
	case "faq":
		return "Спасибо за сообщение. Сейчас коротко отвечу и, если нужно, уточню детали по вашему запросу."
	default:
		if snapshot != nil && snapshot.Positioning != "" {
			return "Спасибо за сообщение. Уточню детали и вернусь с ответом с учётом специфики вашего запроса."
		}
		return "Спасибо за сообщение. Сейчас посмотрю детали и вернусь с ответом."
	}
}

func classifyIntent(text string) intentDecision {
	lower := strings.ToLower(text)
	escalationWords := []string{"жалоб", "претенз", "договор", "гарант", "суд", "верните", "refund", "скидк", "персональн", "не работает"}
	for _, word := range escalationWords {
		if strings.Contains(lower, word) {
			return intentDecision{Intent: "handoff", Confidence: 0.98, Safe: false, Rationale: "В сообщении есть сигналы эскалации или рискованных обещаний."}
		}
	}
	if containsAny(lower, "часы", "время работы", "когда вы", "график", "работаете") {
		return intentDecision{Intent: "hours", Confidence: 0.92, Safe: true, Rationale: "Похоже на вопрос про часы работы."}
	}
	if containsAny(lower, "цена", "стоимость", "сколько стоит", "прайс", "цены") {
		return intentDecision{Intent: "basic_prices", Confidence: 0.9, Safe: true, Rationale: "Похоже на запрос о базовой цене."}
	}
	if containsAny(lower, "интересует", "хочу", "ищу", "нужно", "подскажите", "подобрать") {
		return intentDecision{Intent: "qualification", Confidence: 0.85, Safe: true, Rationale: "Похоже на первичную квалификацию лида."}
	}
	if containsAny(lower, "где", "как", "что", "можно", "есть ли", "какой") {
		return intentDecision{Intent: "faq", Confidence: 0.78, Safe: true, Rationale: "Похоже на FAQ-вопрос."}
	}
	return intentDecision{Intent: "unknown", Confidence: 0.42, Safe: false, Rationale: "Нужен человек для точного ответа."}
}

func (s *VKService) searchKnowledge(ctx context.Context, userID uuid.UUID, query string) []string {
	if strings.TrimSpace(s.cfg.RAGServiceURL) == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]interface{}{
		"query": query,
		"top_k": 3,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.RAGServiceURL, "/")+"/rag/search", bytes.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", userID.String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil
	}
	var payload ragSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	out := []string{}
	for _, item := range payload.Results {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		out = append(out, text)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func (s *VKService) loadInboundMessage(ctx context.Context, userID, integrationID, messageID uuid.UUID) (*models.VKWorkspaceMessage, error) {
	if _, _, err := s.loadIntegration(ctx, userID, integrationID); err != nil {
		return nil, err
	}
	msg := &models.VKWorkspaceMessage{}
	var peerID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, from_vk_user_id, peer_id, COALESCE(text,''), is_incoming, is_processed, received_at
		  FROM vk_messages
		 WHERE id=$1 AND integration_id=$2 AND is_incoming=TRUE`,
		messageID, integrationID,
	).Scan(&msg.ID, &msg.FromVKUserID, &peerID, &msg.Text, &msg.IsIncoming, &msg.IsProcessed, &msg.ReceivedAt)
	if err != nil {
		return nil, fmt.Errorf("not_found: inbound message not found")
	}
	if peerID.Valid {
		msg.PeerID = &peerID.Int64
	}
	return msg, nil
}

func (s *VKService) findDraftByMessage(ctx context.Context, messageID uuid.UUID) (*models.VKReplyDraft, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, integration_id, inbound_message_id, from_vk_user_id, intent, confidence, safe_intent, status, source, draft_text, rationale, knowledge_snippets, sent_message_id, approved_by, generated_at, approved_at, sent_at,
		       orchestration_source, llm_latency_ms, fallback_reason, prompt_tokens, completion_tokens, used_agent_id, used_tools
		  FROM vk_reply_drafts
		 WHERE inbound_message_id=$1`, messageID)
	return scanOneDraft(row)
}

func (s *VKService) loadDraft(ctx context.Context, userID, draftID uuid.UUID) (*models.VKReplyDraft, *models.VKIntegration, error) {
	integ := &models.VKIntegration{}
	var snippetsRaw, toolsRaw []byte
	var sentID sql.NullInt64
	var approvedBy sql.NullString
	var latencyMs, promptTok, completionTok sql.NullInt32
	var fallbackReason sql.NullString
	var usedAgentID uuid.NullUUID
	var orchSource sql.NullString
	var peerID sql.NullInt64
	// group_screen_name and group_photo are nullable in vk_integrations.
	var groupScreenName, groupPhoto sql.NullString
	draft := &models.VKReplyDraft{}

	err := s.db.QueryRowContext(ctx, `
		SELECT d.id, d.integration_id, d.inbound_message_id, d.from_vk_user_id, d.intent, d.confidence, d.safe_intent, d.status, d.source, d.draft_text, d.rationale, d.knowledge_snippets, d.sent_message_id, d.approved_by, d.generated_at, d.approved_at, d.sent_at,
		       d.orchestration_source, d.llm_latency_ms, d.fallback_reason, d.prompt_tokens, d.completion_tokens, d.used_agent_id, d.used_tools, m.peer_id,
		       i.id, i.user_id, i.group_id, i.group_name, i.group_screen_name, i.group_photo, i.is_active, i.connected_at
		  FROM vk_reply_drafts d
		  LEFT JOIN vk_messages m ON m.id=d.inbound_message_id
		  JOIN vk_integrations i ON i.id=d.integration_id
		 WHERE d.id=$1 AND i.user_id=$2 AND i.is_active=TRUE`,
		draftID, userID,
	).Scan(&draft.ID, &draft.IntegrationID, &draft.InboundMessageID, &draft.FromVKUserID, &draft.Intent, &draft.Confidence, &draft.SafeIntent, &draft.Status, &draft.Source, &draft.DraftText, &draft.Rationale, &snippetsRaw, &sentID, &approvedBy, &draft.GeneratedAt, &draft.ApprovedAt, &draft.SentAt,
		&orchSource, &latencyMs, &fallbackReason, &promptTok, &completionTok, &usedAgentID, &toolsRaw, &peerID,
		&integ.ID, &integ.UserID, &integ.GroupID, &integ.GroupName, &groupScreenName, &groupPhoto, &integ.IsActive, &integ.ConnectedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("not_found: draft not found")
	}
	integ.GroupScreenName = groupScreenName.String
	integ.GroupPhoto = groupPhoto.String
	_ = json.Unmarshal(snippetsRaw, &draft.KnowledgeSnippets)
	_ = json.Unmarshal(toolsRaw, &draft.UsedTools)
	if draft.UsedTools == nil {
		draft.UsedTools = []string{}
	}
	if sentID.Valid {
		draft.SentMessageID = &sentID.Int64
	}
	if approvedBy.Valid {
		if uid, err := uuid.Parse(approvedBy.String); err == nil {
			draft.ApprovedBy = &uid
		}
	}
	if orchSource.Valid {
		draft.OrchestrationSource = orchSource.String
	}
	if latencyMs.Valid {
		v := int(latencyMs.Int32)
		draft.LLMLatencyMs = &v
	}
	if fallbackReason.Valid {
		draft.FallbackReason = &fallbackReason.String
	}
	if promptTok.Valid {
		v := int(promptTok.Int32)
		draft.PromptTokens = &v
	}
	if completionTok.Valid {
		v := int(completionTok.Int32)
		draft.CompletionTokens = &v
	}
	if usedAgentID.Valid {
		draft.UsedAgentID = &usedAgentID.UUID
	}
	if peerID.Valid {
		draft.PeerID = &peerID.Int64
	}
	return draft, integ, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, allowing
// scanOneDraft to be called from both single-row and multi-row paths.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanOneDraft reads a single draft row that includes all observability columns.
// The SELECT must include columns in this exact order:
//
//	id, integration_id, inbound_message_id, from_vk_user_id, intent, confidence,
//	safe_intent, status, source, draft_text, rationale, knowledge_snippets,
//	sent_message_id, approved_by, generated_at, approved_at, sent_at,
//	orchestration_source, llm_latency_ms, fallback_reason, prompt_tokens,
//	completion_tokens, used_agent_id, used_tools
func scanOneDraft(row rowScanner) (*models.VKReplyDraft, error) {
	draft := &models.VKReplyDraft{}
	var snippetsRaw, toolsRaw []byte
	var sentID sql.NullInt64
	var approvedBy sql.NullString
	var orchSource sql.NullString
	var latencyMs, promptTok, completionTok sql.NullInt32
	var fallbackReason sql.NullString
	var usedAgentID uuid.NullUUID

	err := row.Scan(
		&draft.ID, &draft.IntegrationID, &draft.InboundMessageID, &draft.FromVKUserID,
		&draft.Intent, &draft.Confidence, &draft.SafeIntent, &draft.Status, &draft.Source,
		&draft.DraftText, &draft.Rationale, &snippetsRaw, &sentID, &approvedBy,
		&draft.GeneratedAt, &draft.ApprovedAt, &draft.SentAt,
		&orchSource, &latencyMs, &fallbackReason, &promptTok, &completionTok,
		&usedAgentID, &toolsRaw,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(snippetsRaw, &draft.KnowledgeSnippets)
	_ = json.Unmarshal(toolsRaw, &draft.UsedTools)
	if draft.UsedTools == nil {
		draft.UsedTools = []string{}
	}
	if sentID.Valid {
		draft.SentMessageID = &sentID.Int64
	}
	if approvedBy.Valid {
		if uid, err := uuid.Parse(approvedBy.String); err == nil {
			draft.ApprovedBy = &uid
		}
	}
	if orchSource.Valid {
		draft.OrchestrationSource = orchSource.String
	}
	if latencyMs.Valid {
		v := int(latencyMs.Int32)
		draft.LLMLatencyMs = &v
	}
	if fallbackReason.Valid {
		draft.FallbackReason = &fallbackReason.String
	}
	if promptTok.Valid {
		v := int(promptTok.Int32)
		draft.PromptTokens = &v
	}
	if completionTok.Valid {
		v := int(completionTok.Int32)
		draft.CompletionTokens = &v
	}
	if usedAgentID.Valid {
		draft.UsedAgentID = &usedAgentID.UUID
	}
	return draft, nil
}

func scanDraftRows(rows *sql.Rows) ([]models.VKReplyDraft, error) {
	out := []models.VKReplyDraft{}
	for rows.Next() {
		draft, err := scanOneDraft(rows)
		if err != nil {
			return nil, err
		}
		if draft != nil {
			out = append(out, *draft)
		}
	}
	return out, rows.Err()
}

func containsAny(text string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func isIntentAllowed(safeIntents []string, intent string) bool {
	for _, item := range safeIntents {
		if item == intent {
			return true
		}
	}
	return false
}
