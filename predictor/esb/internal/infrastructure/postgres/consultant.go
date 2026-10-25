package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/predictboss/predictor/esb/internal/domain"
)

type consultantEventRow struct {
	RequestID      string
	CompanyID      string
	ConversationID string
	AnalysisID     string
	MessageID      string
	Kind           string
	Text           string
	Evidence       []byte
}

const konstantinHistoryCTE = `WITH history_messages AS (
    SELECT m.message_id::text AS message_id, 'assistant'::text AS role, m.kind,
           m.request_id::text AS request_id, m.company_id::text AS company_id,
           m.conversation_id::text AS conversation_id, m.analysis_id::text AS analysis_id,
           m.text, m.evidence, m.created_at
    FROM konstantin_messages m
    WHERE m.company_id = $1::uuid AND m.created_at >= $2 AND m.created_at < $3
    UNION ALL
    SELECT c.call_id::text AS message_id, 'user'::text AS role, 'user_question'::text AS kind,
           c.request_json->>'request_id' AS request_id, v.scope_id AS company_id,
           v.conversation_id::text AS conversation_id, c.request_json->>'analysis_id' AS analysis_id,
           c.request_json->>'text' AS text, '[]'::jsonb AS evidence, c.created_at
    FROM agent_calls c
    JOIN agent_conversations v USING (conversation_id)
    WHERE c.agent_type = 'konstantin' AND v.scope_id = $1::text
      AND c.created_at >= $2 AND c.created_at < $3
      AND c.request_json->>'request_kind' = 'question'
)`

type consultantHistoryRow struct {
	MessageID      string
	Role           string
	Kind           string
	RequestID      string
	CompanyID      string
	ConversationID string
	AnalysisID     string
	Text           string
	Evidence       []byte
	CreatedAt      time.Time
}

func (r *Calls) ConversationID(ctx context.Context, agent domain.AgentType, scopeID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `INSERT INTO agent_conversations (agent_type, scope_id)
VALUES ($1, $2) ON CONFLICT (agent_type, scope_id) DO UPDATE SET updated_at = now()
RETURNING conversation_id::text`, string(agent), scopeID).Scan(&id)
	return id, err
}

func (r *Calls) FreshAnalysis(ctx context.Context, companyID string) (domain.ConsultantEvent, bool, error) {
	row, err := scanConsultantEvent(r.pool.QueryRow(ctx, `SELECT m.request_id::text, m.company_id::text,
m.conversation_id::text, m.analysis_id::text, m.message_id::text, m.kind, m.text, m.evidence, m.created_at
FROM konstantin_messages m
WHERE m.company_id = $1::uuid AND m.kind = 'analysis_result'
  AND COALESCE((SELECT s.profile_synced FROM company_profile_state s WHERE s.company_id = m.company_id), TRUE)
  AND m.profile_revision = COALESCE((SELECT s.profile_revision FROM company_profile_state s WHERE s.company_id = m.company_id), 0)
  AND m.source_revision = COALESCE((SELECT max(n.source_revision) FROM company_news_sources n WHERE n.company_id = m.company_id), 0)
ORDER BY m.created_at DESC, m.message_id DESC LIMIT 1`, companyID))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ConsultantEvent{}, false, nil
	}
	return row, err == nil, err
}

func (r *Calls) Event(ctx context.Context, requestID string) (domain.ConsultantEvent, error) {
	return scanConsultantEvent(r.pool.QueryRow(ctx, `SELECT request_id::text, company_id::text,
conversation_id::text, analysis_id::text, message_id::text, kind, text, evidence, created_at
FROM konstantin_messages WHERE request_id = $1::uuid`, requestID))
}

func (r *Calls) RequestStatus(ctx context.Context, requestID string) (domain.Call, error) {
	var call domain.Call
	err := r.pool.QueryRow(ctx, `SELECT c.call_id::text, c.agent_type, v.scope_id, v.conversation_id::text,
c.request_json, c.status, c.hermes_run_id, c.output, c.error, c.created_at, c.updated_at
FROM agent_calls c JOIN agent_conversations v USING (conversation_id)
WHERE c.agent_type = 'konstantin' AND c.request_json->>'request_id' = $1 LIMIT 1`, requestID).
		Scan(&call.ID, &call.Agent, &call.ScopeID, &call.ConversationID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.CreatedAt, &call.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Call{}, domain.ErrNotFound
	}
	return call, err
}

func (r *Calls) EventsAfter(ctx context.Context, companyID, conversationID, afterMessageID string, limit int) ([]domain.ConsultantEvent, error) {
	if limit < 1 || limit > 10000 {
		limit = 100
	}
	var query string
	var args []any
	if afterMessageID == "" {
		query = `SELECT request_id::text, company_id::text, conversation_id::text, analysis_id::text,
message_id::text, kind, text, evidence, created_at FROM (
  SELECT request_id, company_id, conversation_id, analysis_id, message_id, kind, text, evidence, created_at
  FROM konstantin_messages WHERE company_id = $1::uuid AND conversation_id = $2::uuid
  ORDER BY created_at DESC, message_id DESC LIMIT $3
) recent ORDER BY created_at, message_id`
		args = []any{companyID, conversationID, limit}
	} else {
		var cursorCreatedAt time.Time
		err := r.pool.QueryRow(ctx, `SELECT created_at FROM konstantin_messages WHERE message_id = $1::uuid AND company_id = $2::uuid AND conversation_id = $3::uuid`, afterMessageID, companyID, conversationID).Scan(&cursorCreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		query = `SELECT request_id::text, company_id::text, conversation_id::text, analysis_id::text,
message_id::text, kind, text, evidence, created_at FROM konstantin_messages
WHERE company_id = $1::uuid AND conversation_id = $2::uuid
AND (created_at, message_id) > ($3::timestamptz, $4::uuid)
ORDER BY created_at, message_id LIMIT $5`
		args = []any{companyID, conversationID, cursorCreatedAt, afterMessageID, limit}
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.ConsultantEvent, 0)
	for rows.Next() {
		var row consultantEventRow
		var createdAt time.Time
		if err := rows.Scan(&row.RequestID, &row.CompanyID, &row.ConversationID, &row.AnalysisID, &row.MessageID, &row.Kind, &row.Text, &row.Evidence, &createdAt); err != nil {
			return nil, err
		}
		event, err := row.toEvent(createdAt)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *Calls) KonstantinHistory(ctx context.Context, companyID string, from, to time.Time, beforeMessageID string, limit int) (domain.ConsultantHistoryPage, error) {
	var cursor any
	if beforeMessageID != "" {
		cursor = beforeMessageID
		var found bool
		if err := r.pool.QueryRow(ctx, konstantinHistoryCTE+` SELECT EXISTS (
		    SELECT 1 FROM history_messages WHERE message_id = $4
		)`, companyID, from, to, beforeMessageID).Scan(&found); err != nil {
			return domain.ConsultantHistoryPage{}, err
		}
		if !found {
			return domain.ConsultantHistoryPage{}, domain.ErrNotFound
		}
	}
	rows, err := r.pool.Query(ctx, konstantinHistoryCTE+` SELECT message_id, role, kind, request_id, company_id,
conversation_id, analysis_id, text, evidence, created_at
FROM history_messages
WHERE $4::text IS NULL OR (created_at, message_id) < (
    SELECT created_at, message_id FROM history_messages WHERE message_id = $4
)
ORDER BY created_at DESC, message_id DESC
LIMIT $5`, companyID, from, to, cursor, limit+1)
	if err != nil {
		return domain.ConsultantHistoryPage{}, err
	}
	defer rows.Close()
	messages := make([]domain.ConsultantHistoryMessage, 0, limit+1)
	for rows.Next() {
		var row consultantHistoryRow
		if err := rows.Scan(&row.MessageID, &row.Role, &row.Kind, &row.RequestID, &row.CompanyID,
			&row.ConversationID, &row.AnalysisID, &row.Text, &row.Evidence, &row.CreatedAt); err != nil {
			return domain.ConsultantHistoryPage{}, err
		}
		message := domain.ConsultantHistoryMessage{
			MessageID: row.MessageID, Role: row.Role, Kind: row.Kind, RequestID: row.RequestID,
			CompanyID: row.CompanyID, ConversationID: row.ConversationID, AnalysisID: row.AnalysisID,
			Text: row.Text, CreatedAt: row.CreatedAt,
		}
		if err := json.Unmarshal(row.Evidence, &message.Evidence); err != nil {
			return domain.ConsultantHistoryPage{}, err
		}
		if message.Evidence == nil {
			message.Evidence = []domain.ConsultantEvidence{}
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return domain.ConsultantHistoryPage{}, err
	}
	page := domain.ConsultantHistoryPage{Messages: messages}
	if len(messages) > limit {
		page.HasMore = true
		page.Messages = messages[:limit]
	}
	for left, right := 0, len(page.Messages)-1; left < right; left, right = left+1, right-1 {
		page.Messages[left], page.Messages[right] = page.Messages[right], page.Messages[left]
	}
	if page.HasMore && len(page.Messages) > 0 {
		page.NextBeforeMessageID = page.Messages[0].MessageID
	}
	if page.Messages == nil {
		page.Messages = []domain.ConsultantHistoryMessage{}
	}
	return page, nil
}

func (r *Calls) AnalysisReady(ctx context.Context, companyID, analysisID string) (bool, error) {
	var ready bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (
SELECT 1 FROM konstantin_messages m
WHERE m.company_id = $1::uuid AND m.analysis_id = $2::uuid AND m.kind = 'analysis_result'
  AND COALESCE((SELECT s.profile_synced FROM company_profile_state s WHERE s.company_id = m.company_id), TRUE)
  AND m.profile_revision = COALESCE((SELECT s.profile_revision FROM company_profile_state s WHERE s.company_id = m.company_id), 0)
  AND m.source_revision = COALESCE((SELECT max(n.source_revision) FROM company_news_sources n WHERE n.company_id = m.company_id), 0))`, companyID, analysisID).Scan(&ready)
	return ready, err
}

func (r *Calls) RecordSource(ctx context.Context, source domain.CompanySource) (bool, error) {
	result, err := r.pool.Exec(ctx, `INSERT INTO company_news_sources
(company_id, source_id, news_id, material_kind, media_type, canonical_url, title, published_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, NULLIF($8, '')::timestamptz)
ON CONFLICT (source_id) DO NOTHING`, source.CompanyID, source.SourceID, source.NewsID, source.MaterialKind, source.MediaType, source.SourceURL, source.Title, stringValue(source.PublishedAt))
	if err != nil {
		return false, err
	}
	if result.RowsAffected() == 1 {
		return true, nil
	}
	var existingCompany string
	if err := r.pool.QueryRow(ctx, `SELECT company_id::text FROM company_news_sources WHERE source_id = $1::uuid`, source.SourceID).Scan(&existingCompany); err != nil {
		return false, err
	}
	if existingCompany != source.CompanyID {
		return false, domain.ErrConflict
	}
	return false, nil
}

func (r *Calls) SourceRevision(ctx context.Context, companyID string) (int64, error) {
	var revision int64
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(max(source_revision), 0) FROM company_news_sources WHERE company_id = $1::uuid`, companyID).Scan(&revision)
	return revision, err
}

func (r *Calls) ProfileRevision(ctx context.Context, companyID string) (int64, error) {
	var revision int64
	var synced bool
	err := r.pool.QueryRow(ctx, `SELECT profile_revision, profile_synced FROM company_profile_state WHERE company_id = $1::uuid`, companyID).Scan(&revision, &synced)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	if err == nil && !synced {
		return 0, domain.ErrConflict
	}
	return revision, err
}

type rowScanner interface{ Scan(...any) error }

func scanConsultantEvent(scanner rowScanner) (domain.ConsultantEvent, error) {
	var row consultantEventRow
	var createdAt time.Time
	if err := scanner.Scan(&row.RequestID, &row.CompanyID, &row.ConversationID, &row.AnalysisID, &row.MessageID, &row.Kind, &row.Text, &row.Evidence, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ConsultantEvent{}, domain.ErrNotFound
		}
		return domain.ConsultantEvent{}, err
	}
	return row.toEvent(createdAt)
}

func (row consultantEventRow) toEvent(createdAtTime time.Time) (domain.ConsultantEvent, error) {
	var evidence []domain.ConsultantEvidence
	if err := json.Unmarshal(row.Evidence, &evidence); err != nil {
		return domain.ConsultantEvent{}, err
	}
	if evidence == nil {
		evidence = []domain.ConsultantEvidence{}
	}
	return domain.ConsultantEvent{SchemaVersion: 1, Kind: row.Kind, RequestID: row.RequestID, CompanyID: row.CompanyID,
		ConversationID: row.ConversationID, AnalysisID: row.AnalysisID, MessageID: row.MessageID,
		Text: row.Text, Evidence: evidence, CreatedAt: createdAtTime}, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

var _ domain.CallRepository = (*Calls)(nil)
