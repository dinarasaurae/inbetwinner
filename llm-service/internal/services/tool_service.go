package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	"github.com/google/uuid"
)

type ToolService struct{ db *database.DB }

func NewToolService(db *database.DB) *ToolService { return &ToolService{db: db} }

func (s *ToolService) Create(ctx context.Context, workspaceID uuid.UUID, req models.CreateToolRequest) (*models.Tool, error) {
	if req.Schema == nil {
		req.Schema = json.RawMessage("{}")
	}
	if req.ExecutionConfig == nil {
		req.ExecutionConfig = json.RawMessage("{}")
	}
	if req.TimeoutSecs <= 0 {
		req.TimeoutSecs = 30
	}
	t := &models.Tool{}
	var schemaBytes, execBytes []byte
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO tools (workspace_id, name, display_name, description, type, schema, timeout_seconds, execution_config)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, workspace_id, name, display_name, description, type,
		          schema, is_enabled, timeout_seconds, execution_config, created_at, updated_at`,
		workspaceID, req.Name, req.DisplayName, req.Description,
		req.Type, req.Schema, req.TimeoutSecs, req.ExecutionConfig,
	).Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.DisplayName, &t.Description, &t.Type,
		&schemaBytes, &t.IsEnabled, &t.TimeoutSecs, &execBytes, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert tool: %w", err)
	}
	t.Schema = json.RawMessage(schemaBytes)
	t.ExecutionConfig = json.RawMessage(execBytes)
	return t, nil
}

func (s *ToolService) List(ctx context.Context, workspaceID uuid.UUID) ([]models.Tool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, display_name, description, type,
		       schema, is_enabled, timeout_seconds, execution_config, created_at, updated_at
		FROM tools WHERE workspace_id=$1 ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Tool
	for rows.Next() {
		var t models.Tool
		var schemaBytes, execBytes []byte
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.DisplayName, &t.Description, &t.Type,
			&schemaBytes, &t.IsEnabled, &t.TimeoutSecs, &execBytes, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Schema = json.RawMessage(schemaBytes)
		t.ExecutionConfig = json.RawMessage(execBytes)
		out = append(out, t)
	}
	return out, nil
}

func (s *ToolService) Update(ctx context.Context, id, workspaceID uuid.UUID, req models.CreateToolRequest) (*models.Tool, error) {
	t := &models.Tool{}
	var schemaBytes, execBytes []byte
	err := s.db.QueryRowContext(ctx, `
		UPDATE tools
		SET name=$1, display_name=$2, description=$3, schema=$4,
		    timeout_seconds=$5, execution_config=$6, updated_at=NOW()
		WHERE id=$7 AND workspace_id=$8
		RETURNING id, workspace_id, name, display_name, description, type,
		          schema, is_enabled, timeout_seconds, execution_config, created_at, updated_at`,
		req.Name, req.DisplayName, req.Description, req.Schema,
		req.TimeoutSecs, req.ExecutionConfig, id, workspaceID,
	).Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.DisplayName, &t.Description, &t.Type,
		&schemaBytes, &t.IsEnabled, &t.TimeoutSecs, &execBytes, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update tool: %w", err)
	}
	t.Schema = json.RawMessage(schemaBytes)
	t.ExecutionConfig = json.RawMessage(execBytes)
	return t, nil
}

func (s *ToolService) Delete(ctx context.Context, id, workspaceID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM tools WHERE id=$1 AND workspace_id=$2`, id, workspaceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tool not found")
	}
	return nil
}

func (s *ToolService) GetExecutions(ctx context.Context, workspaceID uuid.UUID, toolName string, limit int) ([]models.ToolExecution, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tool_name, workspace_id, chat_user_id, arguments, status,
		       result, COALESCE(error_message,''), started_at, completed_at, duration_ms
		FROM tool_executions
		WHERE workspace_id=$1 AND tool_name=$2
		ORDER BY started_at DESC LIMIT $3`,
		workspaceID, toolName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ToolExecution
	for rows.Next() {
		var e models.ToolExecution
		var argBytes, resBytes []byte
		if err := rows.Scan(&e.ID, &e.ToolName, &e.WorkspaceID, &e.ChatUserID,
			&argBytes, &e.Status, &resBytes, &e.ErrorMsg,
			&e.StartedAt, &e.CompletedAt, &e.DurationMs); err != nil {
			return nil, err
		}
		e.Arguments = json.RawMessage(argBytes)
		if resBytes != nil {
			e.Result = json.RawMessage(resBytes)
		}
		out = append(out, e)
	}
	return out, nil
}
