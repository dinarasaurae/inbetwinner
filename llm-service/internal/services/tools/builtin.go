package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services/amocrm"
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

var AmoCRMCreateLeadSchema = openai.FunctionDefinition{
	Name:        "create_amocrm_lead",
	Description: "Creates a new lead in AmoCRM (Kommo) CRM system. Use when you have identified a potential client and want to register them in CRM.",
	Parameters: json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string","description":"Lead name, e.g. 'Website request from Acme Corp'"},
			"price":{"type":"integer","description":"Deal value in rubles (optional)"},
			"pipeline_id":{"type":"integer","description":"Pipeline ID (optional, uses default pipeline if omitted)"},
			"contact_name":{"type":"string","description":"Contact person full name"},
			"contact_phone":{"type":"string","description":"Contact phone number"},
			"contact_email":{"type":"string","description":"Contact email address"},
			"notes":{"type":"string","description":"Additional notes about the lead"}
		},
		"required":["name"]
	}`),
}

var AmoCRMCreateTaskSchema = openai.FunctionDefinition{
	Name:        "create_amocrm_task",
	Description: "Creates a task in AmoCRM (Kommo) for a manager to follow up. Use to schedule callbacks, meetings or reminders.",
	Parameters: json.RawMessage(`{
		"type":"object",
		"properties":{
			"text":{"type":"string","description":"Task description, e.g. 'Call back to discuss pricing'"},
			"lead_id":{"type":"integer","description":"ID of the lead to link this task to (optional)"},
			"due_date":{"type":"string","description":"Due datetime ISO 8601, e.g. 2026-04-20T14:00:00+03:00 (default: tomorrow)"}
		},
		"required":["text"]
	}`),
}

var AmoCRMAddNoteSchema = openai.FunctionDefinition{
	Name:        "add_amocrm_note",
	Description: "Adds a text note to an existing lead in AmoCRM. Use to record conversation summary, contact details, or any context that a manager should see.",
	Parameters: json.RawMessage(`{
		"type":"object",
		"properties":{
			"lead_id":{"type":"integer","description":"ID of the lead to attach the note to"},
			"text":{"type":"string","description":"Note text (supports newlines)"}
		},
		"required":["lead_id","text"]
	}`),
}

var AmoCRMGetPipelinesSchema = openai.FunctionDefinition{
	Name:        "get_amocrm_pipelines",
	Description: "Returns the list of sales pipelines and their stages (statuses) in AmoCRM. Call this before create_amocrm_lead when you need to pick the correct pipeline_id.",
	Parameters: json.RawMessage(`{
		"type":"object",
		"properties":{},
		"required":[]
	}`),
}

var CallOperatorSchema = openai.FunctionDefinition{
	Name:        "call_operator",
	Description: "Escalate the conversation to a human operator. Use when you cannot answer the question, need authorisation, or the user explicitly asks to speak with a person.",
	Parameters: json.RawMessage(`{
		"type":"object",
		"properties":{
			"reason":{"type":"string","description":"Why the conversation needs to be escalated to a human operator"}
		},
		"required":["reason"]
	}`),
}

type BuiltinHandler struct {
	calSvc    *calendar.Service
	amoCRMSvc *amocrm.Service
	db        *database.DB
}

func NewBuiltinHandler(calSvc *calendar.Service, amoCRMSvc *amocrm.Service, db *database.DB) *BuiltinHandler {
	return &BuiltinHandler{calSvc: calSvc, amoCRMSvc: amoCRMSvc, db: db}
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

func (h *BuiltinHandler) HandleAmoCRMCreateLead(ctx context.Context, workspaceID uuid.UUID, argsJSON json.RawMessage) (string, error) {
	if h.amoCRMSvc == nil {
		return "", fmt.Errorf("AmoCRM не подключён")
	}
	var p amocrm.CreateLeadParams
	if err := json.Unmarshal(argsJSON, &p); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	result, err := h.amoCRMSvc.CreateLead(ctx, workspaceID, p)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

func (h *BuiltinHandler) HandleAmoCRMCreateTask(ctx context.Context, workspaceID uuid.UUID, argsJSON json.RawMessage) (string, error) {
	if h.amoCRMSvc == nil {
		return "", fmt.Errorf("AmoCRM не подключён")
	}
	var p amocrm.CreateTaskParams
	if err := json.Unmarshal(argsJSON, &p); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	result, err := h.amoCRMSvc.CreateTask(ctx, workspaceID, p)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

func (h *BuiltinHandler) HandleAmoCRMAddNote(ctx context.Context, workspaceID uuid.UUID, argsJSON json.RawMessage) (string, error) {
	if h.amoCRMSvc == nil {
		return "", fmt.Errorf("AmoCRM не подключён")
	}
	var p amocrm.AddNoteParams
	if err := json.Unmarshal(argsJSON, &p); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	result, err := h.amoCRMSvc.AddNote(ctx, workspaceID, p)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

func (h *BuiltinHandler) HandleAmoCRMGetPipelines(ctx context.Context, workspaceID uuid.UUID) (string, error) {
	if h.amoCRMSvc == nil {
		return "", fmt.Errorf("AmoCRM не подключён")
	}
	pipelines, err := h.amoCRMSvc.GetPipelines(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(pipelines)
	return string(out), nil
}
