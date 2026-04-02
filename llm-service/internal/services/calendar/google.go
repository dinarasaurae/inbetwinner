package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googlecalendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type Service struct {
	db          *database.DB
	oauthConfig *oauth2.Config
}

type CreateEventParams struct {
	CalendarID  string   `json:"calendar_id"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	StartTime   string   `json:"start_time"`
	EndTime     string   `json:"end_time"`
	Attendees   []string `json:"attendees"`
	CreateMeet  bool     `json:"create_meet"`
}

type EventResult struct {
	EventID  string `json:"event_id"`
	EventURL string `json:"event_url"`
	MeetURL  string `json:"meet_url,omitempty"`
}

type EventSummary struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Start   string `json:"start"`
	End     string `json:"end"`
	MeetURL string `json:"meet_url,omitempty"`
}

type ListEventsParams struct {
	CalendarID string `json:"calendar_id" query:"calendar_id"`
	TimeMin    string `json:"time_min" query:"time_min"`
	TimeMax    string `json:"time_max" query:"time_max"`
	MaxResults int64  `json:"max_results" query:"max_results"`
}

func NewService(db *database.DB, clientID, clientSecret, redirectURL string) *Service {
	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{googlecalendar.CalendarScope},
		Endpoint:     google.Endpoint,
	}
	return &Service{db: db, oauthConfig: cfg}
}

func (s *Service) GetAuthURL(workspaceID uuid.UUID) string {
	return s.oauthConfig.AuthCodeURL(workspaceID.String(), oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

func (s *Service) HandleCallback(ctx context.Context, code, state string) (*models.GoogleIntegration, error) {
	token, err := s.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange: %w", err)
	}
	workspaceID, err := uuid.Parse(state)
	if err != nil {
		return nil, fmt.Errorf("invalid state: %w", err)
	}

	tokenSource := s.oauthConfig.TokenSource(ctx, token)
	calSvc, err := googlecalendar.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, fmt.Errorf("calendar service: %w", err)
	}
	calList, err := calSvc.CalendarList.List().Do()
	email := ""
	if err == nil && len(calList.Items) > 0 {
		email = calList.Items[0].Id
	}

	integration := &models.GoogleIntegration{}
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO google_integrations (workspace_id, access_token, refresh_token, token_expiry, email, calendar_id)
		VALUES ($1,$2,$3,$4,$5,'primary')
		ON CONFLICT (workspace_id) DO UPDATE SET
			access_token=EXCLUDED.access_token, refresh_token=EXCLUDED.refresh_token,
			token_expiry=EXCLUDED.token_expiry, email=EXCLUDED.email, updated_at=NOW()
		RETURNING id, workspace_id, token_expiry, email, calendar_id, is_active, created_at, updated_at`,
		workspaceID, token.AccessToken, token.RefreshToken, token.Expiry, email,
	).Scan(&integration.ID, &integration.WorkspaceID, &integration.TokenExpiry, &integration.Email,
		&integration.CalendarID, &integration.IsActive, &integration.CreatedAt, &integration.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("save integration: %w", err)
	}
	return integration, nil
}

func (s *Service) getCalendarService(ctx context.Context, workspaceID uuid.UUID) (*googlecalendar.Service, error) {
	var accessToken, refreshToken string
	var expiry time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT access_token, refresh_token, token_expiry FROM google_integrations WHERE workspace_id=$1 AND is_active=true`,
		workspaceID).Scan(&accessToken, &refreshToken, &expiry)
	if err != nil {
		return nil, fmt.Errorf("no google integration: %w", err)
	}
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       expiry,
	}
	tokenSource := s.oauthConfig.TokenSource(ctx, token)
	newToken, _ := tokenSource.Token()
	if newToken != nil && newToken.AccessToken != accessToken {
		_, _ = s.db.ExecContext(ctx, `UPDATE google_integrations SET access_token=$1, token_expiry=$2 WHERE workspace_id=$3`,
			newToken.AccessToken, newToken.Expiry, workspaceID)
	}
	return googlecalendar.NewService(ctx, option.WithTokenSource(tokenSource))
}

func (s *Service) CreateEvent(ctx context.Context, workspaceID uuid.UUID, params CreateEventParams) (*EventResult, error) {
	svc, err := s.getCalendarService(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	event := &googlecalendar.Event{
		Summary:     params.Summary,
		Description: params.Description,
		Start:       &googlecalendar.EventDateTime{DateTime: params.StartTime, TimeZone: "Europe/Moscow"},
		End:         &googlecalendar.EventDateTime{DateTime: params.EndTime, TimeZone: "Europe/Moscow"},
	}
	for _, email := range params.Attendees {
		event.Attendees = append(event.Attendees, &googlecalendar.EventAttendee{Email: email})
	}
	if params.CreateMeet {
		event.ConferenceData = &googlecalendar.ConferenceData{
			CreateRequest: &googlecalendar.CreateConferenceRequest{
				RequestId:             uuid.New().String(),
				ConferenceSolutionKey: &googlecalendar.ConferenceSolutionKey{Type: "hangoutsMeet"},
			},
		}
	}
	calID := params.CalendarID
	if calID == "" {
		calID = "primary"
	}
	call := svc.Events.Insert(calID, event)
	if params.CreateMeet {
		call = call.ConferenceDataVersion(1)
	}
	created, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("insert event: %w", err)
	}
	res := &EventResult{EventID: created.Id, EventURL: created.HtmlLink}
	if created.ConferenceData != nil && len(created.ConferenceData.EntryPoints) > 0 {
		res.MeetURL = created.ConferenceData.EntryPoints[0].Uri
	}
	return res, nil
}

func (s *Service) ListEvents(ctx context.Context, workspaceID uuid.UUID, params ListEventsParams) ([]EventSummary, error) {
	svc, err := s.getCalendarService(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	calID := params.CalendarID
	if calID == "" {
		calID = "primary"
	}
	maxResults := params.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}
	timeMin := params.TimeMin
	if timeMin == "" {
		timeMin = time.Now().Format(time.RFC3339)
	}
	call := svc.Events.List(calID).
		TimeMin(timeMin).
		MaxResults(maxResults).
		SingleEvents(true).
		OrderBy("startTime")
	if params.TimeMax != "" {
		call = call.TimeMax(params.TimeMax)
	}
	events, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	var out []EventSummary
	for _, e := range events.Items {
		es := EventSummary{ID: e.Id, Summary: e.Summary}
		if e.Start != nil {
			es.Start = e.Start.DateTime
		}
		if e.End != nil {
			es.End = e.End.DateTime
		}
		if e.ConferenceData != nil && len(e.ConferenceData.EntryPoints) > 0 {
			es.MeetURL = e.ConferenceData.EntryPoints[0].Uri
		}
		out = append(out, es)
	}
	return out, nil
}

func (s *Service) GetIntegration(ctx context.Context, workspaceID uuid.UUID) (*models.GoogleIntegration, error) {
	g := &models.GoogleIntegration{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, token_expiry, email, calendar_id, is_active, created_at, updated_at
		 FROM google_integrations WHERE workspace_id=$1`, workspaceID,
	).Scan(&g.ID, &g.WorkspaceID, &g.TokenExpiry, &g.Email, &g.CalendarID, &g.IsActive, &g.CreatedAt, &g.UpdatedAt)
	return g, err
}
