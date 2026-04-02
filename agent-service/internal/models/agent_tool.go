package models

import "time"

type AgentTool struct {
	AgentID   string    `json:"agent_id"`
	ToolID    string    `json:"tool_id"`
	ToolName  string    `json:"tool_name"`
	Priority  int       `json:"priority"`
	IsEnabled bool      `json:"is_enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// BuiltinTools — список встроенных инструментов, доступных для назначения агентам
var BuiltinTools = []struct {
	ID          string
	Name        string
	Description string
}{
	{"builtin-update_google_calendar", "update_google_calendar", "Создание и обновление событий в Google Calendar"},
	{"builtin-list_google_calendar", "list_google_calendar", "Просмотр предстоящих событий в Google Calendar"},
	{"builtin-save_contact_info", "save_contact_info", "Сохранение контактной информации из переписки"},
}
