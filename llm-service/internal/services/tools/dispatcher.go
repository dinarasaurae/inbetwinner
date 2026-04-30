package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

type Dispatcher struct {
	db         *database.DB
	builtin    *BuiltinHandler
	registry   *Registry
	httpClient *http.Client
}

func NewDispatcher(db *database.DB, builtin *BuiltinHandler, registry *Registry) *Dispatcher {
	return &Dispatcher{
		db:         db,
		builtin:    builtin,
		registry:   registry,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *Dispatcher) Execute(ctx context.Context, workspaceID uuid.UUID, chatUserID, platform, toolName string, argsJSON json.RawMessage) (string, error) {
	start := time.Now()
	execID := uuid.New()

	_, _ = d.db.ExecContext(ctx, `
		INSERT INTO tool_executions (id, tool_name, workspace_id, chat_user_id, arguments, status)
		VALUES ($1,$2,$3,$4,$5,'running')`,
		execID, toolName, workspaceID, chatUserID, argsJSON)

	var result string
	var execErr error

	switch toolName {
	case "update_google_calendar":
		result, execErr = d.builtin.HandleCalendarCreate(ctx, workspaceID, argsJSON)
	case "list_google_calendar":
		result, execErr = d.builtin.HandleCalendarList(ctx, workspaceID, argsJSON)
	case "save_contact_info":
		result, execErr = d.builtin.HandleSaveContact(ctx, workspaceID, chatUserID, platform, argsJSON)
	case "create_amocrm_lead":
		result, execErr = d.builtin.HandleAmoCRMCreateLead(ctx, workspaceID, argsJSON)
	case "create_amocrm_task":
		result, execErr = d.builtin.HandleAmoCRMCreateTask(ctx, workspaceID, argsJSON)
	case "add_amocrm_note":
		result, execErr = d.builtin.HandleAmoCRMAddNote(ctx, workspaceID, argsJSON)
	case "get_amocrm_pipelines":
		result, execErr = d.builtin.HandleAmoCRMGetPipelines(ctx, workspaceID)
	case "create_zoho_lead":
		result, execErr = d.builtin.HandleZohoCreateLead(ctx, workspaceID, argsJSON)
	case "create_zoho_task":
		result, execErr = d.builtin.HandleZohoCreateTask(ctx, workspaceID, argsJSON)
	case "add_zoho_note":
		result, execErr = d.builtin.HandleZohoAddNote(ctx, workspaceID, argsJSON)
	case "get_zoho_deal_stages":
		result, execErr = d.builtin.HandleZohoGetDealStages(ctx, workspaceID)
	case "call_operator":
		// The LLMService detects this tool in usedTools and sends the AGENT_STUCK push.
		// Here we just return a confirmation so the LLM can craft a polite message to the user.
		result = `{"escalated":true,"message":"Оператор уведомлён и скоро свяжется с вами"}`
	default:
		tool, _ := d.registry.GetToolByName(ctx, workspaceID, toolName)
		if tool == nil {
			execErr = fmt.Errorf("unknown tool: %s", toolName)
		} else if tool.Type == models.ToolTypeHTTPWebhook {
			result, execErr = d.executeHTTPWebhook(ctx, tool, argsJSON)
		} else {
			execErr = fmt.Errorf("unhandled tool type for: %s", toolName)
		}
	}

	durationMs := int(time.Since(start).Milliseconds())
	now := time.Now()
	if execErr != nil {
		_, _ = d.db.ExecContext(ctx,
			`UPDATE tool_executions SET status='failed', error_message=$1, completed_at=$2, duration_ms=$3 WHERE id=$4`,
			execErr.Error(), now, durationMs, execID)
		return "", execErr
	}
	resultJSON, _ := json.Marshal(result)
	_, _ = d.db.ExecContext(ctx,
		`UPDATE tool_executions SET status='success', result=$1, completed_at=$2, duration_ms=$3 WHERE id=$4`,
		resultJSON, now, durationMs, execID)
	return result, nil
}

func (d *Dispatcher) executeHTTPWebhook(ctx context.Context, tool *models.Tool, argsJSON json.RawMessage) (string, error) {
	var cfg struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	_ = json.Unmarshal(tool.ExecutionConfig, &cfg)
	if cfg.URL == "" {
		return "", fmt.Errorf("no URL in execution_config")
	}
	timeout := time.Duration(tool.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.URL, bytes.NewReader(argsJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return string(body), nil
}

func (d *Dispatcher) ToToolMessage(call openai.ToolCall, result string) openai.ChatCompletionMessage {
	return openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		Content:    result,
		ToolCallID: call.ID,
	}
}
