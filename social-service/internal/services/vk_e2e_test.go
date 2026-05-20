//go:build integration

package services

// Integration tests for the VK inbound → draft → approve/send production path.
//
// Prerequisites:
//   - A running Postgres instance with social-service migrations applied.
//   - Environment variable INTEGRATION_TEST_DB set to the DSN, e.g.:
//       postgres://social_user:social_password@localhost:5432/social_db?sslmode=disable
//
// Run with:
//   go test -tags integration -v ./internal/services/ -run TestE2E
//
// Each test seeds its own isolated fixtures (unique user_id per test) and
// cleans up after itself.  Tests are safe to run in parallel.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
	"github.com/dinarasaurae/inbetwin-social-service/internal/crypto"
	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
	vkpkg "github.com/dinarasaurae/inbetwin-social-service/internal/vk"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// testEncKey is a 32-byte AES key used for all integration test fixtures.
var testEncKey = []byte(strings.Repeat("k", 32))

// openIntegrationDB opens a Postgres connection to the test database.
// The test is skipped when INTEGRATION_TEST_DB is not set.
func openIntegrationDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := os.Getenv("INTEGRATION_TEST_DB")
	if dsn == "" {
		t.Skip("INTEGRATION_TEST_DB not set — skipping integration test")
	}

	raw, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := raw.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	// Run migrations so the schema is always current.
	migrationsDir := resolveProjectRoot(t, "migrations")
	driver, err := migratepostgres.WithInstance(raw, &migratepostgres.Config{})
	if err != nil {
		t.Fatalf("migrate driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "postgres", driver)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}

	return &database.DB{DB: raw}
}

// resolveProjectRoot walks up from the test file to find the social-service root.
func resolveProjectRoot(t *testing.T, subdir string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	// filename: .../social-service/internal/services/vk_e2e_test.go
	root := filepath.Dir(filepath.Dir(filepath.Dir(filename))) // → social-service/
	return filepath.Join(root, subdir)
}

// testConfig returns a minimal Config for VKService construction.
func testConfig(llmServiceURL, orchMode string) *config.Config {
	return &config.Config{
		LLMServiceURL:        llmServiceURL,
		LLMOrchestrationMode: orchMode,
		LLMProvider:          "openai",
		LLMAPIKey:            "sk-test",
	}
}

// e2eFixtures holds IDs seeded for one test run.
type e2eFixtures struct {
	userID  uuid.UUID
	integID uuid.UUID
	msgID   uuid.UUID
	draftID uuid.UUID // populated by seedDraft
}

// seedBase creates a VK integration + agent settings + one inbound message.
// Each call uses a fresh userID so tests are isolated.
func seedBase(t *testing.T, db *database.DB, enc *crypto.Encryptor, orchMode string, autoReply bool) e2eFixtures {
	t.Helper()
	ctx := context.Background()
	f := e2eFixtures{
		userID:  uuid.New(),
		integID: uuid.New(),
		msgID:   uuid.New(),
	}

	// Encrypt a fake group token.
	tokenEnc, tokenIV, err := enc.Encrypt([]byte("fake_group_token_for_testing"))
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}

	// Insert vk_integration.
	_, err = db.ExecContext(ctx, `
		INSERT INTO vk_integrations
			(id, user_id, group_id, group_token_enc, group_token_iv, group_name, is_active)
		VALUES ($1,$2,999999,$3,$4,'E2E Test Group',TRUE)`,
		f.integID, f.userID, tokenEnc, tokenIV,
	)
	if err != nil {
		t.Fatalf("seed integration: %v", err)
	}

	// Insert vk_agent_settings with the requested orchestration_mode.
	_, err = db.ExecContext(ctx, `
		INSERT INTO vk_agent_settings
			(integration_id, draft_first, auto_reply_enabled,
			 safe_intents, tone_of_voice, escalation_policy,
			 rag_enabled, orchestration_mode)
		VALUES ($1, TRUE, $2,
			ARRAY['faq','basic_prices']::TEXT[], 'friendly', 'escalate to human',
			TRUE, $3)`,
		f.integID, autoReply, orchMode,
	)
	if err != nil {
		t.Fatalf("seed agent settings: %v", err)
	}

	// Insert inbound vk_message.
	_, err = db.ExecContext(ctx, `
		INSERT INTO vk_messages
			(id, integration_id, from_vk_user_id, message_id,
			 text, is_incoming, is_processed, received_at)
		VALUES ($1,$2,777,1001,'Сколько стоит ваш продукт?',TRUE,FALSE,NOW())`,
		f.msgID, f.integID,
	)
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}

	t.Cleanup(func() { cleanupFixtures(t, db, f) })
	return f
}

// seedDraft inserts a pending draft linked to f.msgID and sets f.draftID.
func seedDraft(t *testing.T, db *database.DB, f *e2eFixtures, draftText string) {
	t.Helper()
	f.draftID = uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO vk_reply_drafts
			(id, integration_id, inbound_message_id, from_vk_user_id,
			 intent, confidence, safe_intent, status, source,
			 draft_text, rationale, generated_at)
		VALUES ($1,$2,$3,777,
			'faq',0.90,TRUE,'pending','llm_service',
			$4,'product pricing query',NOW())`,
		f.draftID, f.integID, f.msgID, draftText,
	)
	if err != nil {
		t.Fatalf("seed draft: %v", err)
	}
}

// cleanupFixtures removes all rows belonging to this test's user.
func cleanupFixtures(t *testing.T, db *database.DB, f e2eFixtures) {
	t.Helper()
	ctx := context.Background()
	// Delete in dependency order (drafts → messages → settings → integration).
	db.ExecContext(ctx, `DELETE FROM vk_reply_drafts WHERE integration_id=$1`, f.integID)   //nolint
	db.ExecContext(ctx, `DELETE FROM vk_messages WHERE integration_id=$1`, f.integID)       //nolint
	db.ExecContext(ctx, `DELETE FROM vk_agent_settings WHERE integration_id=$1`, f.integID) //nolint
	db.ExecContext(ctx, `DELETE FROM vk_integrations WHERE id=$1`, f.integID)               //nolint
}

// mockLLMServer spins up an httptest server that returns the given decision JSON.
func mockLLMServer(t *testing.T, decision *models.VKOrchestrationDecision) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/llm/social/vk/process" {
			http.Error(w, "unexpected path", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(decision) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv
}

// mockLLMServerError returns an httptest server that always returns 503.
func mockLLMServerError(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// mockVKAPIServer spins up an httptest server that handles messages.send.
// It returns the given messageID as the VK response.
func mockVKAPIServer(t *testing.T, returnMsgID int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"response": %d}`, returnMsgID)
	}))
	t.Cleanup(func() {
		srv.Close()
		// Restore production VK API base after test.
		vkpkg.APIBase = "https://api.vk.com/method"
	})
	return srv
}

// buildVKService constructs a VKService backed by the test DB and encryptor.
func buildVKService(db *database.DB, enc *crypto.Encryptor, cfg *config.Config) *VKService {
	return NewVKService(db, enc, cfg)
}

// queryDraftByMsg fetches the draft created for the given inbound message.
func queryDraftByMsg(t *testing.T, db *database.DB, msgID uuid.UUID) *models.VKReplyDraft {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	draft, err := (&VKService{db: db}).findDraftByMessage(ctx, msgID)
	if err != nil {
		t.Fatalf("findDraftByMessage: %v", err)
	}
	return draft
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestE2E_LLMService_Success_DraftPersisted verifies the happy path:
// llm-service returns a draft decision → draft row persisted in vk_reply_drafts
// with orchestration_source=llm_service and all observability columns populated.
func TestE2E_LLMService_Success_DraftPersisted(t *testing.T) {
	db := openIntegrationDB(t)
	enc, err := crypto.NewEncryptor(testEncKey)
	if err != nil {
		t.Fatal(err)
	}

	agentID := uuid.New()
	decision := &models.VKOrchestrationDecision{
		Mode:              "draft",
		DraftText:         "Наш базовый тариф стоит 5 000 ₽/мес.",
		Confidence:        0.92,
		Intent:            "basic_prices",
		SafeIntent:        true,
		Rationale:         "Вопрос о базовом тарифе — safe intent",
		KnowledgeSnippets: []string{"Тариф Базовый: 5 000 ₽/мес"},
		UsedTools:         []string{},
		PromptTokens:      85,
		CompletionTokens:  32,
		TokensUsed:        117,
		AgentID:           &agentID,
	}

	llmSrv := mockLLMServer(t, decision)
	f := seedBase(t, db, enc, "llm_service", false)
	cfg := testConfig(llmSrv.URL, "llm_service")
	svc := buildVKService(db, enc, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	svc.processInboundMessage(ctx, f.userID, f.integID, f.msgID)

	draft := queryDraftByMsg(t, db, f.msgID)

	if draft.DraftText != decision.DraftText {
		t.Errorf("draft_text: got %q, want %q", draft.DraftText, decision.DraftText)
	}
	if draft.Intent != decision.Intent {
		t.Errorf("intent: got %q, want %q", draft.Intent, decision.Intent)
	}
	if !draft.SafeIntent {
		t.Error("safe_intent should be true")
	}
	if draft.Status != models.VKDraftStatusPending {
		t.Errorf("status: got %q, want %q", draft.Status, models.VKDraftStatusPending)
	}
	if draft.OrchestrationSource != "llm_service" {
		t.Errorf("orchestration_source: got %q, want llm_service", draft.OrchestrationSource)
	}
	// Mock httptest responds in <1 ms, so Milliseconds() may return 0;
	// we only require the column to be set (not NULL).
	if draft.LLMLatencyMs == nil {
		t.Error("llm_latency_ms should be set (non-NULL)")
	}
	if draft.PromptTokens == nil || *draft.PromptTokens != 85 {
		t.Errorf("prompt_tokens: got %v, want 85", draft.PromptTokens)
	}
	if draft.UsedAgentID == nil || *draft.UsedAgentID != agentID {
		t.Error("used_agent_id mismatch")
	}

	// Inbound message must be marked processed.
	var processed bool
	db.QueryRowContext(ctx, `SELECT is_processed FROM vk_messages WHERE id=$1`, f.msgID).Scan(&processed) //nolint
	if !processed {
		t.Error("vk_message.is_processed should be TRUE after processing")
	}
}

// TestE2E_Hybrid_Error_FallbackLegacy verifies that when the llm-service
// returns a non-2xx response in hybrid mode, the legacy path runs and produces
// a draft with orchestration_source=fallback_legacy and fallback_reason set.
func TestE2E_Hybrid_Error_FallbackLegacy(t *testing.T) {
	db := openIntegrationDB(t)
	enc, err := crypto.NewEncryptor(testEncKey)
	if err != nil {
		t.Fatal(err)
	}

	llmSrv := mockLLMServerError(t)
	f := seedBase(t, db, enc, "hybrid", false)
	cfg := testConfig(llmSrv.URL, "hybrid")
	svc := buildVKService(db, enc, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	svc.processInboundMessage(ctx, f.userID, f.integID, f.msgID)

	draft := queryDraftByMsg(t, db, f.msgID)

	if draft.OrchestrationSource != "fallback_legacy" {
		t.Errorf("orchestration_source: got %q, want fallback_legacy", draft.OrchestrationSource)
	}
	if draft.FallbackReason == nil || *draft.FallbackReason == "" {
		t.Error("fallback_reason should be set when llm-service fails")
	}
	if draft.DraftText == "" {
		t.Error("fallback_legacy draft should still produce draft text")
	}
}

// TestE2E_ApproveDraft_OutboundSend verifies the approve → send path:
// given a pending draft, ApproveAndSendDraft sends the message via VK API
// and updates the draft status to "sent" with sent_message_id and approved_at.
func TestE2E_ApproveDraft_OutboundSend(t *testing.T) {
	db := openIntegrationDB(t)
	enc, err := crypto.NewEncryptor(testEncKey)
	if err != nil {
		t.Fatal(err)
	}

	// Mock VK API server that returns message ID 42.
	const fakeSentMsgID int64 = 42
	vkSrv := mockVKAPIServer(t, fakeSentMsgID)
	// Point VK API client at the mock server.
	vkpkg.APIBase = vkSrv.URL

	f := seedBase(t, db, enc, "legacy", false)
	seedDraft(t, db, &f, "Добро пожаловать! Готов ответить на ваши вопросы.")

	cfg := testConfig("", "legacy")
	svc := buildVKService(db, enc, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := svc.ApproveAndSendDraft(ctx, f.userID, f.draftID, "")
	if err != nil {
		t.Fatalf("ApproveAndSendDraft: %v", err)
	}

	if result.Status != models.VKDraftStatusSent {
		t.Errorf("status: got %q, want %q", result.Status, models.VKDraftStatusSent)
	}
	if result.SentMessageID == nil || *result.SentMessageID != fakeSentMsgID {
		t.Errorf("sent_message_id: got %v, want %d", result.SentMessageID, fakeSentMsgID)
	}
	if result.ApprovedAt == nil {
		t.Error("approved_at should be set after approval")
	}
	if result.SentAt == nil {
		t.Error("sent_at should be set after sending")
	}
	if result.ApprovedBy == nil || *result.ApprovedBy != f.userID {
		t.Errorf("approved_by: got %v, want %s", result.ApprovedBy, f.userID)
	}
}

// TestE2E_ApproveDraft_WithTextOverride verifies that the optional textOverride
// replaces the original draft text in the outbound message.
func TestE2E_ApproveDraft_WithTextOverride(t *testing.T) {
	db := openIntegrationDB(t)
	enc, err := crypto.NewEncryptor(testEncKey)
	if err != nil {
		t.Fatal(err)
	}

	const fakeSentMsgID int64 = 77
	vkSrv := mockVKAPIServer(t, fakeSentMsgID)
	vkpkg.APIBase = vkSrv.URL

	f := seedBase(t, db, enc, "legacy", false)
	seedDraft(t, db, &f, "Original draft text.")

	cfg := testConfig("", "legacy")
	svc := buildVKService(db, enc, cfg)

	override := "Исправленный текст ответа."
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := svc.ApproveAndSendDraft(ctx, f.userID, f.draftID, override)
	if err != nil {
		t.Fatalf("ApproveAndSendDraft: %v", err)
	}
	// The persisted draft_text should reflect the override.
	if result.DraftText != override {
		t.Errorf("draft_text after override: got %q, want %q", result.DraftText, override)
	}
}

// TestE2E_AutoReply_SafeIntent_AutoSent verifies that when the llm-service
// returns mode=auto_reply and the integration has auto_reply_enabled=TRUE,
// the message is sent immediately without human approval and the draft status
// is auto_sent.
func TestE2E_AutoReply_SafeIntent_AutoSent(t *testing.T) {
	db := openIntegrationDB(t)
	enc, err := crypto.NewEncryptor(testEncKey)
	if err != nil {
		t.Fatal(err)
	}

	const fakeSentMsgID int64 = 99
	vkSrv := mockVKAPIServer(t, fakeSentMsgID)
	vkpkg.APIBase = vkSrv.URL

	decision := &models.VKOrchestrationDecision{
		Mode:             "auto_reply", // triggers auto-send
		DraftText:        "Наш офис работает с 9 до 18, пн–пт.",
		Confidence:       0.97,
		Intent:           "hours",
		SafeIntent:       true,
		Rationale:        "Safe intent: hours of operation",
		UsedTools:        []string{},
		PromptTokens:     40,
		CompletionTokens: 15,
		TokensUsed:       55,
	}

	llmSrv := mockLLMServer(t, decision)
	// auto_reply_enabled=TRUE so the service sends without approval.
	f := seedBase(t, db, enc, "llm_service", true)
	_, err = db.ExecContext(context.Background(), `
		UPDATE vk_agent_settings
		   SET draft_first=FALSE
		 WHERE integration_id=$1`,
		f.integID,
	)
	if err != nil {
		t.Fatalf("disable draft_first: %v", err)
	}
	cfg := testConfig(llmSrv.URL, "llm_service")
	svc := buildVKService(db, enc, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	svc.processInboundMessage(ctx, f.userID, f.integID, f.msgID)

	draft := queryDraftByMsg(t, db, f.msgID)

	if draft.Status != models.VKDraftStatusAutoSent {
		t.Errorf("status: got %q, want %q", draft.Status, models.VKDraftStatusAutoSent)
	}
	if draft.SentMessageID == nil || *draft.SentMessageID != fakeSentMsgID {
		t.Errorf("sent_message_id: got %v, want %d", draft.SentMessageID, fakeSentMsgID)
	}
	if draft.SentAt == nil {
		t.Error("sent_at should be set for auto_sent draft")
	}
	if draft.Intent != "hours" {
		t.Errorf("intent: got %q, want hours", draft.Intent)
	}
	if draft.OrchestrationSource != "llm_service" {
		t.Errorf("orchestration_source: got %q, want llm_service", draft.OrchestrationSource)
	}
}

// TestE2E_LLMNilClient_FallsBackToLegacy verifies that when no LLM_SERVICE_URL
// is configured (nil client), processInboundMessage always uses the legacy path.
func TestE2E_LLMNilClient_FallsBackToLegacy(t *testing.T) {
	db := openIntegrationDB(t)
	enc, err := crypto.NewEncryptor(testEncKey)
	if err != nil {
		t.Fatal(err)
	}

	// cfg has empty LLM_SERVICE_URL → llmClient will be nil.
	f := seedBase(t, db, enc, "llm_service", false)
	cfg := testConfig("", "llm_service") // mode=llm_service but URL is empty
	svc := buildVKService(db, enc, cfg)

	if svc.llmClient != nil {
		t.Skip("LLM client unexpectedly non-nil; skipping (set LLM_SERVICE_URL='' to reproduce)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	svc.processInboundMessage(ctx, f.userID, f.integID, f.msgID)

	draft := queryDraftByMsg(t, db, f.msgID)
	if draft.OrchestrationSource != "legacy" {
		t.Errorf("expected legacy fallback when LLM client is nil, got %q", draft.OrchestrationSource)
	}
}
