package services

// tg_userbot.go — handles incoming Telegram DMs received via MTProto userbot.
//
// Flow (mirrors VK processInboundMessageLLM):
//   1. Persist the raw inbound message.
//   2. Call llm-service /llm/social/vk/process (platform="telegram").
//   3. If mode == "auto_reply" → send reply DM via MTProto listener.
//   4. Update the DB row with reply text / error.

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/dinarasaurae/inbetwin-social-service/internal/mtproto"
)

// TGReplyFunc sends a reply DM back to the sender.
// Injected from main.go so this service stays decoupled from mtproto package.
type TGReplyFunc func(ctx context.Context, workspaceID uuid.UUID, peerUserID, peerAccessHash int64, text string) error

// TGUserbotService routes incoming Telegram DMs through the AI pipeline.
type TGUserbotService struct {
	db        *database.DB
	cfg       *config.Config
	llmClient *VKLLMClient // reuse same client — endpoint accepts platform field
	replyFn   TGReplyFunc
}

func NewTGUserbotService(
	db *database.DB,
	cfg *config.Config,
	replyFn TGReplyFunc,
) *TGUserbotService {
	return &TGUserbotService{
		db:        db,
		cfg:       cfg,
		llmClient: NewVKLLMClient(cfg.LLMServiceURL),
		replyFn:   replyFn,
	}
}

// HandleDM is the DMHandler registered with mtproto.ListenerManager.
// Called in a goroutine per incoming message — must not block the update loop.
func (s *TGUserbotService) HandleDM(
	ctx context.Context,
	workspaceID uuid.UUID,
	senderUserID int64,
	senderAccessHash int64,
	senderUsername string,
	text string,
) {
	// Give ourselves a generous timeout independent of the update-loop context.
	opCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msgID, err := s.persistInbound(opCtx, workspaceID, senderUserID, senderUsername, text)
	if err != nil {
		log.Printf("[tg-userbot] persist error workspace=%s sender=%d: %v", workspaceID, senderUserID, err)
		return
	}

	decision, err := s.llmClient.ProcessVKMessage(opCtx, workspaceID, VKProcessRequest{
		IntegrationID: workspaceID.String(), // session is per-workspace, no separate integration row
		ChatUserID:    fmt.Sprintf("%d", senderUserID),
		Platform:      "telegram",
		Message:       text,
	})
	if err != nil {
		log.Printf("[tg-userbot] llm-service error workspace=%s: %v", workspaceID, err)
		s.updateError(opCtx, msgID, err.Error())
		return
	}

	mode := decision.Mode
	reply := decision.DraftText

	s.updateDecision(opCtx, msgID, mode, reply)

	if mode == "auto_reply" && reply != "" && s.replyFn != nil {
		if sendErr := s.replyFn(opCtx, workspaceID, senderUserID, senderAccessHash, reply); sendErr != nil {
			log.Printf("[tg-userbot] send DM error workspace=%s: %v", workspaceID, sendErr)
			s.updateError(opCtx, msgID, sendErr.Error())
		} else {
			s.markReplySent(opCtx, msgID)
		}
	}
}

// AsDMHandler returns the HandleDM method as an mtproto.DMHandler.
func (s *TGUserbotService) AsDMHandler() mtproto.DMHandler {
	return s.HandleDM
}

// ── DB helpers ────────────────────────────────────────────────────────────────

func (s *TGUserbotService) persistInbound(ctx context.Context, workspaceID uuid.UUID, senderID int64, senderUsername, text string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO telegram_dm_messages
			(workspace_id, tg_sender_id, tg_sender_username, tg_message_id, text)
		VALUES ($1, $2, $3, 0, $4)
		ON CONFLICT (workspace_id, tg_message_id) DO NOTHING
		RETURNING id`,
		workspaceID,
		senderID,
		nullableStr(senderUsername),
		text,
	).Scan(&id)
	if err == sql.ErrNoRows {
		// Duplicate — already processed.
		return uuid.Nil, fmt.Errorf("duplicate message")
	}
	return id, err
}

func (s *TGUserbotService) updateDecision(ctx context.Context, id uuid.UUID, mode, reply string) {
	_, _ = s.db.ExecContext(ctx,
		`UPDATE telegram_dm_messages SET mode=$1, reply_text=$2 WHERE id=$3`,
		mode, nullableStr(reply), id)
}

func (s *TGUserbotService) markReplySent(ctx context.Context, id uuid.UUID) {
	_, _ = s.db.ExecContext(ctx,
		`UPDATE telegram_dm_messages SET reply_sent_at=NOW() WHERE id=$1`, id)
}

func (s *TGUserbotService) updateError(ctx context.Context, id uuid.UUID, errMsg string) {
	_, _ = s.db.ExecContext(ctx,
		`UPDATE telegram_dm_messages SET reply_error=$1 WHERE id=$2`, errMsg, id)
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
