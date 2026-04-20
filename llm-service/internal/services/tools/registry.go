package tools

import (
	"context"
	"encoding/json"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

type AvailableTool struct {
	Name       string
	Definition openai.FunctionDefinition
	Type       models.ToolType
	ToolID     string
}

type Registry struct{ db *database.DB }

func NewRegistry(db *database.DB) *Registry { return &Registry{db: db} }

func (r *Registry) HasGoogleCalendar(ctx context.Context, workspaceID uuid.UUID) bool {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM google_integrations WHERE workspace_id=$1 AND is_active=true`,
		workspaceID).Scan(&count)
	return err == nil && count > 0
}

func (r *Registry) HasAmoCRM(ctx context.Context, workspaceID uuid.UUID) bool {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM amocrm_integrations WHERE workspace_id=$1 AND is_active=true`,
		workspaceID).Scan(&count)
	return err == nil && count > 0
}

// GetTools returns the tools available for a given workspace.
//
// allowedToolIDs — when non-nil, only tools whose ToolID appears in this set
// are returned. Pass nil to allow everything (no agent restriction, dev/fallback).
// Built-in calendar/CRM tools are additionally gated on the respective integration flags.
func (r *Registry) GetTools(ctx context.Context, workspaceID uuid.UUID, hasGoogleCal bool, allowedToolIDs map[string]bool) ([]AvailableTool, error) {
	var out []AvailableTool

	allowed := func(id string) bool {
		if allowedToolIDs == nil {
			return true
		}
		return allowedToolIDs[id]
	}

	if hasGoogleCal && allowed("builtin-calendar-create") {
		out = append(out, AvailableTool{Name: "update_google_calendar", Definition: CalendarCreateSchema, Type: models.ToolTypeBuiltin, ToolID: "builtin-calendar-create"})
	}
	if hasGoogleCal && allowed("builtin-calendar-list") {
		out = append(out, AvailableTool{Name: "list_google_calendar", Definition: CalendarListSchema, Type: models.ToolTypeBuiltin, ToolID: "builtin-calendar-list"})
	}
	if allowed("builtin-save-contact") {
		out = append(out, AvailableTool{Name: "save_contact_info", Definition: SaveContactSchema, Type: models.ToolTypeBuiltin, ToolID: "builtin-save-contact"})
	}
	// AmoCRM tools — gated on active integration
	hasAmoCRM := r.HasAmoCRM(ctx, workspaceID)
	if hasAmoCRM && allowed("builtin-amocrm-get-pipelines") {
		out = append(out, AvailableTool{Name: "get_amocrm_pipelines", Definition: AmoCRMGetPipelinesSchema, Type: models.ToolTypeBuiltin, ToolID: "builtin-amocrm-get-pipelines"})
	}
	if hasAmoCRM && allowed("builtin-amocrm-create-lead") {
		out = append(out, AvailableTool{Name: "create_amocrm_lead", Definition: AmoCRMCreateLeadSchema, Type: models.ToolTypeBuiltin, ToolID: "builtin-amocrm-create-lead"})
	}
	if hasAmoCRM && allowed("builtin-amocrm-create-task") {
		out = append(out, AvailableTool{Name: "create_amocrm_task", Definition: AmoCRMCreateTaskSchema, Type: models.ToolTypeBuiltin, ToolID: "builtin-amocrm-create-task"})
	}
	if hasAmoCRM && allowed("builtin-amocrm-add-note") {
		out = append(out, AvailableTool{Name: "add_amocrm_note", Definition: AmoCRMAddNoteSchema, Type: models.ToolTypeBuiltin, ToolID: "builtin-amocrm-add-note"})
	}

	// call_operator is always available — it's a safety valve for every agent.
	if allowed("builtin-call-operator") {
		out = append(out, AvailableTool{Name: "call_operator", Definition: CallOperatorSchema, Type: models.ToolTypeBuiltin, ToolID: "builtin-call-operator"})
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, type, schema
		FROM tools WHERE workspace_id=$1 AND is_enabled=true ORDER BY name`,
		workspaceID)
	if err != nil {
		return out, nil
	}
	defer rows.Close()
	for rows.Next() {
		var t models.Tool
		var schemaBytes []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Type, &schemaBytes); err != nil {
			continue
		}
		if !allowed(t.ID.String()) {
			continue
		}
		t.Schema = json.RawMessage(schemaBytes)
		out = append(out, AvailableTool{
			Name: t.Name,
			Definition: openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
			Type:   t.Type,
			ToolID: t.ID.String(),
		})
	}
	return out, nil
}

func (r *Registry) GetToolByName(ctx context.Context, workspaceID uuid.UUID, name string) (*models.Tool, error) {
	t := &models.Tool{}
	var schemaBytes, execBytes []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, display_name, description, type,
		       schema, is_enabled, timeout_seconds, execution_config, created_at, updated_at
		FROM tools WHERE workspace_id=$1 AND name=$2`,
		workspaceID, name,
	).Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.DisplayName, &t.Description, &t.Type,
		&schemaBytes, &t.IsEnabled, &t.TimeoutSecs, &execBytes, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, nil //nolint — not found is OK, caller handles nil
	}
	t.Schema = json.RawMessage(schemaBytes)
	t.ExecutionConfig = json.RawMessage(execBytes)
	return t, nil
}

func ToOpenAITools(ts []AvailableTool) []openai.Tool {
	out := make([]openai.Tool, len(ts))
	for i, t := range ts {
		def := t.Definition
		out[i] = openai.Tool{Type: openai.ToolTypeFunction, Function: &def}
	}
	return out
}
