package history

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

type Service struct{ db *database.DB }

func NewService(db *database.DB) *Service { return &Service{db: db} }

func (s *Service) GetHistory(ctx context.Context, workspaceID uuid.UUID, chatUserID string, limit int) ([]models.ChatMessage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, chat_user_id, platform, role, content,
		       COALESCE(tool_name,''), COALESCE(tool_call_id,''), total_tokens, created_at
		FROM chat_messages
		WHERE workspace_id=$1 AND chat_user_id=$2
		ORDER BY created_at DESC LIMIT $3`, workspaceID, chatUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []models.ChatMessage
	for rows.Next() {
		var m models.ChatMessage
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.ChatUserID, &m.Platform, &m.Role,
			&m.Content, &m.ToolName, &m.ToolCallID, &m.TotalTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	// reverse so chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (s *Service) AppendMessage(ctx context.Context, msg models.ChatMessage) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chat_messages (workspace_id, chat_user_id, platform, role, content, tool_name, tool_call_id, total_tokens)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		msg.WorkspaceID, msg.ChatUserID, msg.Platform, msg.Role, msg.Content,
		sql.NullString{String: msg.ToolName, Valid: msg.ToolName != ""},
		sql.NullString{String: msg.ToolCallID, Valid: msg.ToolCallID != ""},
		msg.TotalTokens)
	return err
}

func (s *Service) ClearHistory(ctx context.Context, workspaceID uuid.UUID, chatUserID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chat_messages WHERE workspace_id=$1 AND chat_user_id=$2`, workspaceID, chatUserID)
	return err
}

func (s *Service) ToOpenAIMessages(msgs []models.ChatMessage) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		msg := openai.ChatCompletionMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
		if m.ToolCallID != "" {
			msg.ToolCallID = m.ToolCallID
		}
		out = append(out, msg)
	}
	return out
}

func (s *Service) GetCount(ctx context.Context, workspaceID uuid.UUID, chatUserID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_messages WHERE workspace_id=$1 AND chat_user_id=$2`, workspaceID, chatUserID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get count: %w", err)
	}
	return count, nil
}
