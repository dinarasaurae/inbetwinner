package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-lead-scoring-service/internal/database"
	"github.com/dinarasaurae/inbetwin-lead-scoring-service/internal/models"
)

var signalWeights = map[string]int{
	models.SignalAskedPrice:        10,
	models.SignalAskedTimeline:     8,
	models.SignalMentionedBudget:   12,
	models.SignalRequestedDemo:     15,
	models.SignalMentionedCompany:  5,
	models.SignalLongMessage:       3,
	models.SignalPositiveSentiment: 5,
	models.SignalNegativeSentiment: -15,
	models.SignalQuestion:          3,
}

// keyword lists for signal detection (Russian + English)
var signalKeywords = map[string][]string{
	models.SignalAskedPrice: {
		"цена", "цены", "стоимость", "сколько стоит", "сколько стоит", "прайс",
		"тариф", "тарифы", "расценки", "price", "cost", "how much", "pricing",
	},
	models.SignalAskedTimeline: {
		"сроки", "срок", "когда", "как скоро", "сколько времени", "дедлайн",
		"timeline", "deadline", "when", "how long", "timeframe",
	},
	models.SignalMentionedBudget: {
		"бюджет", "бюджета", "могу потратить", "готов заплатить", "располагаем",
		"budget", "afford", "spend", "investment",
	},
	models.SignalRequestedDemo: {
		"демо", "демонстрация", "показать", "покажите", "презентация",
		"demo", "demonstration", "show me", "trial", "test drive",
	},
	models.SignalMentionedCompany: {
		"компания", "организация", "команда", "наш бизнес", "наша фирма",
		"company", "organization", "our team", "our business", "enterprise",
	},
	models.SignalPositiveSentiment: {
		"отлично", "хорошо", "интересно", "нравится", "круто", "замечательно",
		"подходит", "согласен", "excellent", "great", "perfect", "love", "awesome",
		"interested", "sounds good",
	},
	models.SignalNegativeSentiment: {
		"не интересно", "дорого", "не подходит", "откажусь", "не буду",
		"not interested", "too expensive", "no thanks", "cancel", "refund",
	},
	models.SignalQuestion: {"?"},
}

type ScorerService struct {
	db               *database.DB
	hotLeadThreshold int
	notifyURL        string
}

func NewScorerService(db *database.DB, hotLeadThreshold int, notifyURL string) *ScorerService {
	return &ScorerService{db: db, hotLeadThreshold: hotLeadThreshold, notifyURL: notifyURL}
}

func (s *ScorerService) Score(ctx context.Context, req models.ScoreRequest) (*models.ScoreResponse, error) {
	signals := s.detectSignals(req.Message)

	// long message bonus
	if len([]rune(req.Message)) > 200 {
		signals = append(signals, models.SignalLongMessage)
	}

	delta := 0
	for _, sig := range signals {
		delta += signalWeights[sig]
	}

	current, err := s.getOrCreateScore(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get score: %w", err)
	}

	newScore := clamp(current.Score+delta, 0, 100)

	// persist signals
	for _, sig := range signals {
		if err := s.saveSignal(ctx, req.WorkspaceID, req.LeadID, sig, signalWeights[sig], req.Message); err != nil {
			// non-fatal
			_ = err
		}
	}

	// update lead score
	label := scoreLabel(newScore)
	_, err = s.db.ExecContext(ctx,
		`UPDATE lead_scores SET score=$1, score_label=$2, last_message=$3, message_count=message_count+1, updated_at=NOW()
         WHERE workspace_id=$4 AND lead_id=$5`,
		newScore, label, req.Message, req.WorkspaceID, req.LeadID,
	)
	if err != nil {
		return nil, fmt.Errorf("update score: %w", err)
	}

	return &models.ScoreResponse{
		LeadID:     req.LeadID,
		Score:      newScore,
		ScoreLabel: label,
		Delta:      delta,
		IsHot:      newScore >= s.hotLeadThreshold,
		Signals:    signals,
	}, nil
}

func (s *ScorerService) GetScore(ctx context.Context, workspaceID uuid.UUID, leadID string) (*models.LeadScore, error) {
	var ls models.LeadScore
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, lead_id, platform, score, score_label, last_message, message_count, updated_at, created_at
         FROM lead_scores WHERE workspace_id=$1 AND lead_id=$2`,
		workspaceID, leadID,
	).Scan(&ls.ID, &ls.WorkspaceID, &ls.LeadID, &ls.Platform, &ls.Score, &ls.ScoreLabel,
		&ls.LastMessage, &ls.MessageCount, &ls.UpdatedAt, &ls.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &ls, err
}

func (s *ScorerService) ListScores(ctx context.Context, workspaceID uuid.UUID, limit, offset int) ([]models.LeadScore, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, lead_id, platform, score, score_label, last_message, message_count, updated_at, created_at
         FROM lead_scores WHERE workspace_id=$1 ORDER BY score DESC LIMIT $2 OFFSET $3`,
		workspaceID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.LeadScore
	for rows.Next() {
		var ls models.LeadScore
		if err := rows.Scan(&ls.ID, &ls.WorkspaceID, &ls.LeadID, &ls.Platform, &ls.Score, &ls.ScoreLabel,
			&ls.LastMessage, &ls.MessageCount, &ls.UpdatedAt, &ls.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, ls)
	}
	return result, rows.Err()
}

func (s *ScorerService) GetSignals(ctx context.Context, workspaceID uuid.UUID, leadID string) ([]models.BehavioralSignal, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, lead_id, signal_type, weight, message, created_at
         FROM behavioral_signals WHERE workspace_id=$1 AND lead_id=$2 ORDER BY created_at DESC LIMIT 50`,
		workspaceID, leadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.BehavioralSignal
	for rows.Next() {
		var sig models.BehavioralSignal
		if err := rows.Scan(&sig.ID, &sig.WorkspaceID, &sig.LeadID, &sig.SignalType, &sig.Weight, &sig.Message, &sig.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, sig)
	}
	return result, rows.Err()
}

// --- helpers ---

func (s *ScorerService) detectSignals(message string) []string {
	lower := strings.ToLower(message)
	var found []string
	seen := map[string]bool{}
	for sigType, keywords := range signalKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				if !seen[sigType] {
					found = append(found, sigType)
					seen[sigType] = true
				}
				break
			}
		}
	}
	return found
}

func (s *ScorerService) getOrCreateScore(ctx context.Context, req models.ScoreRequest) (*models.LeadScore, error) {
	var ls models.LeadScore
	err := s.db.QueryRowContext(ctx,
		`SELECT id, score FROM lead_scores WHERE workspace_id=$1 AND lead_id=$2`,
		req.WorkspaceID, req.LeadID,
	).Scan(&ls.ID, &ls.Score)
	if err == sql.ErrNoRows {
		ls.ID = uuid.New()
		ls.Score = 0
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO lead_scores (id, workspace_id, lead_id, platform, score, score_label, last_message, message_count)
             VALUES ($1,$2,$3,$4,0,'cold','',0)`,
			ls.ID, req.WorkspaceID, req.LeadID, req.Platform,
		)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return &ls, nil
}

func (s *ScorerService) saveSignal(ctx context.Context, workspaceID uuid.UUID, leadID, sigType string, weight int, message string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO behavioral_signals (id, workspace_id, lead_id, signal_type, weight, message, created_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		uuid.New(), workspaceID, leadID, sigType, weight, truncate(message, 500), time.Now(),
	)
	return err
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func scoreLabel(score int) string {
	switch {
	case score >= 80:
		return "hot"
	case score >= 50:
		return "warm"
	case score >= 20:
		return "lukewarm"
	default:
		return "cold"
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
