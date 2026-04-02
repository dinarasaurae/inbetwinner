package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services/calendar"
	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

var CalendarCreateSchema = openai.FunctionDefinition{
	Name:        "update_google_calendar",
	Description: "Creates or updates a meeting/event in Google Calendar. Use when user wants to schedule, book, or arrange a meeting.",
	Parameters: json.RawMessage(`{
		"type":"object",
		"properties":{
			"summary":{"type":"string","description":"Meeting title"},
			"start_time":{"type":"string","description":"Start datetime ISO 8601, e.g. 2024-01-15T14:00:00+03:00"},
			"end_time":{"type":"string","description":"End datetime ISO 8601"},
			"description":{"type":"string","description":"Meeting description or agenda"},
			"attendees":{"type":"array","items":{"type":"string"},"description":"Attendee email addresses"},
			"create_meet":{"type":"boolean","description":"Create Google Meet video link"},
			"calendar_id":{"type":"string","description":"Calendar ID, default is primary"}
		},
		"required":["summary","start_time","end_time"]
	}`),
}

var CalendarListSchema = openai.FunctionDefinition{
	Name:        "list_google_calendar",
	Description: "Lists upcoming calendar events to check availability or show schedule.",
	Parameters: json.RawMessage(`{
		"type":"object",
		"properties":{
			"time_min":{"type":"string","description":"Start time ISO 8601 (default: now)"},
			"time_max":{"type":"string","description":"End time ISO 8601 (optional)"},
			"max_results":{"type":"integer","description":"Max events to return (default 10)"},
			"calendar_id":{"type":"string","description":"Calendar ID, default primary"}
		},
		"required":[]
	}`),
}

var SaveContactSchema = openai.FunctionDefinition{
	Name:        "save_contact_info",
	Description: "Saves contact information extracted from the conversation to the database.",
	Parameters: json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string","description":"Contact full name"},
			"email":{"type":"string","description":"Email address"},
			"phone":{"type":"string","description":"Phone number"},
			"company":{"type":"string","description":"Company name"},
			"notes":{"type":"string","description":"Additional notes about this contact"}
		},
		"required":[]
	}`),
}

type BuiltinHandler struct {
	calSvc *calendar.Service
	db     *database.DB
}

func NewBuiltinHandler(calSvc *calendar.Service, db *database.DB) *BuiltinHandler {
	return &BuiltinHandler{calSvc: calSvc, db: db}
}

func (h *BuiltinHandler) HandleCalendarCreate(ctx context.Context, workspaceID uuid.UUID, argsJSON json.RawMessage) (string, error) {
	if h.calSvc == nil {
		return "", fmt.Errorf("Google Calendar не подключён")
	}
	var p calendar.CreateEventParams
	if err := json.Unmarshal(argsJSON, &p); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	result, err := h.calSvc.CreateEvent(ctx, workspaceID, p)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

func (h *BuiltinHandler) HandleCalendarList(ctx context.Context, workspaceID uuid.UUID, argsJSON json.RawMessage) (string, error) {
	if h.calSvc == nil {
		return "", fmt.Errorf("Google Calendar не подключён")
	}
	var p calendar.ListEventsParams
	_ = json.Unmarshal(argsJSON, &p)
	events, err := h.calSvc.ListEvents(ctx, workspaceID, p)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(events)
	return string(out), nil
}

func (h *BuiltinHandler) HandleSaveContact(ctx context.Context, workspaceID uuid.UUID, chatUserID, platform string, argsJSON json.RawMessage) (string, error) {
	var args struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Phone   string `json:"phone"`
		Company string `json:"company"`
		Notes   string `json:"notes"`
	}
	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return "", err
	}
	_, err := h.db.ExecContext(ctx, `
		INSERT INTO contacts (workspace_id, chat_user_id, platform, name, email, phone, company, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (workspace_id, chat_user_id) DO UPDATE SET
			name=COALESCE(NULLIF(EXCLUDED.name,''), contacts.name),
			email=COALESCE(NULLIF(EXCLUDED.email,''), contacts.email),
			phone=COALESCE(NULLIF(EXCLUDED.phone,''), contacts.phone),
			company=COALESCE(NULLIF(EXCLUDED.company,''), contacts.company),
			notes=COALESCE(NULLIF(EXCLUDED.notes,''), contacts.notes),
			updated_at=NOW()`,
		workspaceID, chatUserID, platform,
		args.Name, args.Email, args.Phone, args.Company, args.Notes)
	if err != nil {
		return "", err
	}
	return `{"message":"Контактная информация сохранена"}`, nil
}
