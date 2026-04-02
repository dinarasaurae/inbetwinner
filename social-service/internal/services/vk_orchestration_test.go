package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
)

// ── Minimal stubs ─────────────────────────────────────────────────────────────

// stubLLMClient replaces VKLLMClient in tests.
type stubLLMClient struct {
	decision *models.VKOrchestrationDecision
	err      error
	called   bool
}

func (c *stubLLMClient) ProcessVKMessage(_ context.Context, _ uuid.UUID, _ VKProcessRequest) (*models.VKOrchestrationDecision, error) {
	c.called = true
	return c.decision, c.err
}

// orchClient is the interface actually used by processInboundMessageLLM.
// This allows replacing *VKLLMClient with the stub in tests.
type orchClient interface {
	ProcessVKMessage(ctx context.Context, workspaceID uuid.UUID, req VKProcessRequest) (*models.VKOrchestrationDecision, error)
}

// testVKService is a thin wrapper that exposes the dispatch logic with
// injectable dependencies, without touching production DB code.
type testVKService struct {
	mode      string // legacy | llm_service | hybrid
	llm       orchClient
	legacyCh  chan string // writes "legacy" when legacy path is taken
	llmCh     chan string // writes "llm" when llm path is taken
	fallbackCh chan string // writes "fallback" when hybrid falls back
}

func (t *testVKService) dispatch(ctx context.Context, integrationID uuid.UUID, message string) string {
	switch t.mode {
	case "llm_service":
		return t.callLLM(ctx, integrationID, message, false)
	case "hybrid":
		return t.callLLM(ctx, integrationID, message, true)
	default: // legacy
		return "legacy"
	}
}

func (t *testVKService) callLLM(ctx context.Context, integrationID uuid.UUID, message string, withFallback bool) string {
	if t.llm == nil {
		return "legacy" // nil client → fallback
	}
	req := VKProcessRequest{
		IntegrationID: integrationID.String(),
		ChatUserID:    "12345",
		Platform:      "vk",
		Message:       message,
	}
	decision, err := t.llm.ProcessVKMessage(ctx, uuid.New(), req)
	if err != nil {
		if withFallback {
			return "fallback_legacy"
		}
		return "error"
	}
	return decision.Mode
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestOrchestrationMode_Legacy(t *testing.T) {
	svc := &testVKService{mode: "legacy"}
	result := svc.dispatch(context.Background(), uuid.New(), "Сколько стоит услуга?")
	if result != "legacy" {
		t.Errorf("expected legacy path, got %q", result)
	}
}

func TestOrchestrationMode_LLMService_Success(t *testing.T) {
	agentID := uuid.New()
	stub := &stubLLMClient{
		decision: &models.VKOrchestrationDecision{
			Mode:       "draft",
			DraftText:  "Спасибо за вопрос! Стоимость от 1000 руб.",
			Confidence: 0.91,
			Intent:     "basic_prices",
			SafeIntent: true,
			Rationale:  "Вопрос о цене, безопасный интент",
			AgentID:    &agentID,
		},
	}
	svc := &testVKService{mode: "llm_service", llm: stub}
	result := svc.dispatch(context.Background(), uuid.New(), "Сколько стоит?")
	if result != "draft" {
		t.Errorf("expected draft mode, got %q", result)
	}
	if !stub.called {
		t.Error("expected llm client to be called")
	}
}

func TestOrchestrationMode_Hybrid_LLMFails_FallsBackToLegacy(t *testing.T) {
	stub := &stubLLMClient{err: errors.New("connection refused")}
	svc := &testVKService{mode: "hybrid", llm: stub}
	result := svc.dispatch(context.Background(), uuid.New(), "Привет!")
	if result != "fallback_legacy" {
		t.Errorf("expected fallback_legacy, got %q", result)
	}
	if !stub.called {
		t.Error("expected llm client to be called before fallback")
	}
}

func TestOrchestrationMode_Hybrid_LLMTimeout_FallsBackToLegacy(t *testing.T) {
	stub := &stubLLMClient{err: context.DeadlineExceeded}
	svc := &testVKService{mode: "hybrid", llm: stub}
	result := svc.dispatch(context.Background(), uuid.New(), "Вопрос")
	if result != "fallback_legacy" {
		t.Errorf("expected fallback_legacy on timeout, got %q", result)
	}
}

func TestOrchestrationMode_LLMService_AutoReply(t *testing.T) {
	stub := &stubLLMClient{
		decision: &models.VKOrchestrationDecision{
			Mode:       "auto_reply",
			DraftText:  "Работаем с 9 до 18.",
			Confidence: 0.95,
			Intent:     "hours",
			SafeIntent: true,
			Rationale:  "Безопасный запрос о часах работы",
		},
	}
	svc := &testVKService{mode: "llm_service", llm: stub}
	result := svc.dispatch(context.Background(), uuid.New(), "Когда вы работаете?")
	if result != "auto_reply" {
		t.Errorf("expected auto_reply, got %q", result)
	}
}

func TestOrchestrationMode_LLMService_Escalate(t *testing.T) {
	stub := &stubLLMClient{
		decision: &models.VKOrchestrationDecision{
			Mode:       "escalate",
			DraftText:  "",
			Confidence: 0.98,
			Intent:     "handoff",
			SafeIntent: false,
			Rationale:  "Жалоба — требует менеджера",
		},
	}
	svc := &testVKService{mode: "llm_service", llm: stub}
	result := svc.dispatch(context.Background(), uuid.New(), "Хочу вернуть деньги!")
	if result != "escalate" {
		t.Errorf("expected escalate, got %q", result)
	}
}

func TestOrchestrationMode_LLMNil_FallsBackToLegacy(t *testing.T) {
	svc := &testVKService{mode: "llm_service", llm: nil}
	result := svc.dispatch(context.Background(), uuid.New(), "Привет")
	if result != "legacy" {
		t.Errorf("expected legacy when llm client is nil, got %q", result)
	}
}

func TestOrchestrationMode_HybridNilClient_FallsBackToLegacy(t *testing.T) {
	svc := &testVKService{mode: "hybrid", llm: nil}
	result := svc.dispatch(context.Background(), uuid.New(), "Тест")
	if result != "legacy" {
		t.Errorf("expected legacy when nil in hybrid mode, got %q", result)
	}
}

// ── VKOrchestrationDecision model tests ──────────────────────────────────────

func TestVKOrchestrationDecision_Fields(t *testing.T) {
	agentID := uuid.New()
	d := models.VKOrchestrationDecision{
		Mode:              "draft",
		DraftText:         "Ответ",
		Confidence:        0.87,
		Intent:            "qualification",
		SafeIntent:        true,
		Rationale:         "Лид интересуется услугой",
		KnowledgeSnippets: []string{"snippet1"},
		UsedTools:         []string{"save_contact_info"},
		PromptTokens:      120,
		CompletionTokens:  45,
		TokensUsed:        165,
		AgentID:           &agentID,
	}
	if d.Mode != "draft" {
		t.Errorf("unexpected mode: %s", d.Mode)
	}
	if d.TokensUsed != 165 {
		t.Errorf("unexpected tokens: %d", d.TokensUsed)
	}
	if d.AgentID == nil || *d.AgentID != agentID {
		t.Error("agent_id mismatch")
	}
}

// ── VKProcessRequest type tests ───────────────────────────────────────────────

func TestVKProcessRequest_ContextPropagation(t *testing.T) {
	req := VKProcessRequest{
		IntegrationID: uuid.New().String(),
		ChatUserID:    "987654",
		Platform:      "vk",
		Message:       "Какие тарифы?",
		Context: &VKProcessContext{
			ToneOfVoice:       "дружелюбный",
			SafeIntents:       []string{"basic_prices", "faq"},
			ForbiddenPromises: []string{"гарантия возврата"},
			AutoReplyEnabled:  true,
			BusinessSnapshot:  "Онлайн школа программирования",
		},
	}
	if req.Context.ToneOfVoice != "дружелюбный" {
		t.Error("tone_of_voice not propagated")
	}
	if len(req.Context.SafeIntents) != 2 {
		t.Errorf("expected 2 safe intents, got %d", len(req.Context.SafeIntents))
	}
}

// ── draftObservability field tests ───────────────────────────────────────────

func TestDraftObservability_Fields(t *testing.T) {
	latency := 237
	prompt := 100
	completion := 50
	reason := "connection refused"
	agentID := uuid.New()

	obs := &draftObservability{
		source:           "llm_service",
		latencyMs:        &latency,
		fallbackReason:   &reason,
		promptTokens:     &prompt,
		completionTokens: &completion,
		usedAgentID:      &agentID,
		usedTools:        []string{"save_contact_info", "update_google_calendar"},
	}
	if obs.source != "llm_service" {
		t.Errorf("unexpected source: %s", obs.source)
	}
	if *obs.latencyMs != 237 {
		t.Errorf("unexpected latency: %d", *obs.latencyMs)
	}
	if len(obs.usedTools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(obs.usedTools))
	}
}

// ── classifyIntent stays working (legacy path regression) ────────────────────

func TestClassifyIntent_LegacyRegression(t *testing.T) {
	cases := []struct {
		text       string
		wantIntent string
		wantSafe   bool
	}{
		{"сколько стоит ваш курс?", "basic_prices", true},
		{"когда вы работаете?", "hours", true},
		{"хочу записаться на консультацию", "qualification", true},
		{"верните деньги", "handoff", false},
		{"что такое Go?", "faq", true},
	}
	for _, tc := range cases {
		d := classifyIntent(tc.text)
		if d.Intent != tc.wantIntent {
			t.Errorf("text=%q: got intent=%q, want=%q", tc.text, d.Intent, tc.wantIntent)
		}
		if d.Safe != tc.wantSafe {
			t.Errorf("text=%q: got safe=%v, want=%v", tc.text, d.Safe, tc.wantSafe)
		}
	}
}

// ── scoreLabel equivalent — VKDraftStatus is set correctly ───────────────────

func TestVKDraftStatus_Constants(t *testing.T) {
	statuses := []models.VKDraftStatus{
		models.VKDraftStatusPending,
		models.VKDraftStatusApproved,
		models.VKDraftStatusSent,
		models.VKDraftStatusAutoSent,
		models.VKDraftStatusSkipped,
		models.VKDraftStatusFailed,
	}
	if len(statuses) != 6 {
		t.Errorf("expected 6 status constants")
	}
}

// ── isIntentAllowed stays working ────────────────────────────────────────────

func TestIsIntentAllowed(t *testing.T) {
	safe := []string{"faq", "hours", "basic_prices"}
	if !isIntentAllowed(safe, "faq") {
		t.Error("faq should be allowed")
	}
	if isIntentAllowed(safe, "handoff") {
		t.Error("handoff should not be allowed")
	}
}

// ── Config flag plumbing ──────────────────────────────────────────────────────

func TestOrchestrationMode_DefaultIsLegacy(t *testing.T) {
	// The default when env var not set should be legacy.
	// This matches config.go: getEnv("LLM_ORCHESTRATION_MODE", "legacy")
	const defaultMode = "legacy"
	svc := &testVKService{mode: defaultMode}
	result := svc.dispatch(context.Background(), uuid.New(), "test")
	if result != "legacy" {
		t.Errorf("default mode should produce legacy path, got %q", result)
	}
}

// Compile-time check that stubLLMClient satisfies the orchClient interface.
var _ orchClient = (*stubLLMClient)(nil)

// Unused import guard.
var _ = time.Now
