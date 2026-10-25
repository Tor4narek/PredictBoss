package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/predictboss/predictor/esb/internal/domain"
)

type Calls struct{ pool *pgxpool.Pool }

func NewCalls(pool *pgxpool.Pool) *Calls { return &Calls{pool: pool} }

func (r *Calls) Enqueue(ctx context.Context, agent domain.AgentType, scopeID string, input json.RawMessage, idempotencyKey string) (returned domain.Call, reused bool, resultErr error) {
	if len(idempotencyKey) > 255 || !visibleASCII(idempotencyKey) {
		return domain.Call{}, false, domain.ErrConflict
	}
	if idempotencyKey != "" {
		call, sameRequest, err := r.byIdempotency(ctx, idempotencyKey, input)
		if err == nil {
			if !sameRequest || call.Agent != agent || call.ScopeID != scopeID {
				return domain.Call{}, false, domain.ErrConflict
			}
			if call.Agent == domain.AgentSerezha && (call.Status == domain.CallFailed || call.Status == domain.CallInterrupted || call.Status == domain.CallCancelled) {
				requeued, ok, retryErr := r.requeueFailedNews(ctx, call.ID)
				if retryErr != nil {
					return domain.Call{}, false, retryErr
				}
				if ok {
					return requeued, true, nil
				}
				// Another identical retry may have already requeued the call.
				call, sameRequest, err = r.byIdempotency(ctx, idempotencyKey, input)
				if err != nil {
					return domain.Call{}, false, err
				}
				if !sameRequest {
					return domain.Call{}, false, domain.ErrConflict
				}
			}
			return call, true, nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return domain.Call{}, false, err
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Call{}, false, err
	}
	defer func() { resultErr = errors.Join(resultErr, rollbackTransaction(ctx, tx)) }()
	var conversationID string
	err = tx.QueryRow(ctx, `INSERT INTO agent_conversations (agent_type, scope_id)
VALUES ($1, $2)
ON CONFLICT (agent_type, scope_id) DO UPDATE SET updated_at = now()
RETURNING conversation_id::text`, string(agent), scopeID).Scan(&conversationID)
	if err != nil {
		return domain.Call{}, false, err
	}
	var call domain.Call
	call.Agent, call.ScopeID, call.ConversationID, call.Input, call.Status = agent, scopeID, conversationID, input, domain.CallQueued
	err = tx.QueryRow(ctx, `INSERT INTO agent_calls (conversation_id, agent_type, request_json, idempotency_key, status)
VALUES ($1, $2, $3::jsonb, NULLIF($4, ''), 'queued')
RETURNING call_id::text, created_at, updated_at`, conversationID, string(agent), input, idempotencyKey).
		Scan(&call.ID, &call.CreatedAt, &call.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.Call{}, false, domain.ErrConflict
		}
		return domain.Call{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO agent_messages (conversation_id, call_id, role, content) VALUES ($1, $2, 'user', $3)`, conversationID, call.ID, string(input)); err != nil {
		return domain.Call{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Call{}, false, err
	}
	return call, false, nil
}

func (r *Calls) byIdempotency(ctx context.Context, key string, input json.RawMessage) (domain.Call, bool, error) {
	var call domain.Call
	var sameRequest bool
	err := r.pool.QueryRow(ctx, `SELECT c.call_id::text, c.agent_type, v.scope_id, v.conversation_id::text, c.request_json, c.status,
c.hermes_run_id, c.output, c.error, c.retry_count, c.created_at, c.updated_at, c.request_json = $2::jsonb
FROM agent_calls c JOIN agent_conversations v USING (conversation_id) WHERE c.idempotency_key = $1`, key, input).
		Scan(&call.ID, &call.Agent, &call.ScopeID, &call.ConversationID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.RetryCount, &call.CreatedAt, &call.UpdatedAt, &sameRequest)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Call{}, false, domain.ErrNotFound
	}
	if err == nil && call.Agent == domain.AgentSerezha {
		sameRequest = sameNewsRequest(call.Input, input)
	}
	return call, sameRequest, err
}

func sameNewsRequest(stored, submitted []byte) bool {
	normalize := func(raw []byte) ([]byte, bool) {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return nil, false
		}
		if rawText, ok := fields["news_text"]; ok {
			var text string
			if err := json.Unmarshal(rawText, &text); err != nil {
				return nil, false
			}
			digest := sha256.Sum256([]byte(text))
			encoded, err := json.Marshal(hex.EncodeToString(digest[:]))
			if err != nil {
				return nil, false
			}
			fields["news_text_sha256"] = encoded
			delete(fields, "news_text")
		}
		encoded, err := json.Marshal(fields)
		return encoded, err == nil
	}
	left, leftOK := normalize(stored)
	right, rightOK := normalize(submitted)
	return leftOK && rightOK && string(left) == string(right)
}

func (r *Calls) requeueFailedNews(ctx context.Context, callID string) (domain.Call, bool, error) {
	var call domain.Call
	err := r.pool.QueryRow(ctx, `UPDATE agent_calls c
SET status = 'queued', hermes_run_id = NULL, output = NULL, error = 'news_retry', updated_at = now()
FROM agent_conversations v
WHERE c.call_id = $1 AND c.agent_type = 'serezha' AND c.status IN ('failed','interrupted','cancelled')
  AND v.conversation_id = c.conversation_id
RETURNING c.call_id::text, c.agent_type, v.scope_id, v.conversation_id::text, c.request_json, c.status,
c.hermes_run_id, c.output, c.error, c.retry_count, c.created_at, c.updated_at`, callID).
		Scan(&call.ID, &call.Agent, &call.ScopeID, &call.ConversationID, &call.Input, &call.Status, &call.HermesRunID,
			&call.Output, &call.Error, &call.RetryCount, &call.CreatedAt, &call.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Call{}, false, nil
	}
	return call, err == nil, err
}

func (r *Calls) Get(ctx context.Context, id string) (domain.Call, error) {
	return r.get(ctx, id, "", false)
}

func (r *Calls) GetForAgent(ctx context.Context, id string, agent domain.AgentType) (domain.Call, error) {
	return r.get(ctx, id, agent, true)
}

func (r *Calls) get(ctx context.Context, id string, agent domain.AgentType, constrain bool) (domain.Call, error) {
	query := `SELECT c.call_id::text, c.agent_type, v.scope_id, v.conversation_id::text, c.request_json, c.status, c.hermes_run_id,
c.output, c.error, c.retry_count, c.created_at, c.updated_at FROM agent_calls c JOIN agent_conversations v USING (conversation_id)
WHERE c.call_id = $1`
	if constrain {
		query += ` AND c.agent_type = $2`
	}
	var call domain.Call
	var err error
	if constrain {
		err = r.pool.QueryRow(ctx, query, id, string(agent)).Scan(&call.ID, &call.Agent, &call.ScopeID, &call.ConversationID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.RetryCount, &call.CreatedAt, &call.UpdatedAt)
	} else {
		err = r.pool.QueryRow(ctx, query, id).Scan(&call.ID, &call.Agent, &call.ScopeID, &call.ConversationID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.RetryCount, &call.CreatedAt, &call.UpdatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Call{}, domain.ErrNotFound
	}
	return call, err
}

func (r *Calls) LoadHistory(ctx context.Context, callID string, limit int) ([]domain.Message, error) {
	if limit == 0 {
		return []domain.Message{}, nil
	}
	if limit < 0 {
		return nil, domain.ErrConflict
	}
	rows, err := r.pool.Query(ctx, `SELECT role, content FROM (
SELECT m.message_id, m.role, m.content FROM agent_messages m JOIN agent_calls c USING (conversation_id)
WHERE c.call_id = $1 AND m.call_id <> c.call_id ORDER BY m.message_id DESC LIMIT $2
) recent ORDER BY message_id ASC`, callID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]domain.Message, 0, limit)
	for rows.Next() {
		var message domain.Message
		if err := rows.Scan(&message.Role, &message.Content); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

type contextCursor struct {
	CallID    string
	CreatedAt time.Time
}

type contextHistoryRow struct {
	MessageID int64
	CallID    string
	CreatedAt time.Time
	Role      string
	Content   string
}

// LoadContextWindow reads only turns before the target call. It returns the
// latest whole-call suffix for Hermes and, when the configured threshold is
// crossed, a bounded oldest prefix that can be compacted into the summary.
func (r *Calls) LoadContextWindow(ctx context.Context, callID string, recentLimit, compressAfter, batchMessages int) (domain.ContextWindow, error) {
	if recentLimit < 0 || compressAfter < 0 || batchMessages < 1 {
		return domain.ContextWindow{}, domain.ErrConflict
	}
	var window domain.ContextWindow
	var agentType string
	var targetCreatedAt time.Time
	if err := r.pool.QueryRow(ctx, `SELECT c.conversation_id::text, c.agent_type, c.created_at
FROM agent_calls c WHERE c.call_id = $1`, callID).Scan(&window.ConversationID, &agentType, &targetCreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ContextWindow{}, domain.ErrNotFound
		}
		return domain.ContextWindow{}, err
	}
	var throughAt *time.Time
	var throughID *string
	err := r.pool.QueryRow(ctx, `SELECT summary, through_call_created_at, through_call_id::text
FROM agent_context_summaries WHERE conversation_id = $1::uuid`, window.ConversationID).
		Scan(&window.Summary, &throughAt, &throughID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.ContextWindow{}, err
	}
	if throughAt != nil {
		window.SummaryThroughCreatedAt = *throughAt
	}
	if throughID != nil {
		window.SummaryThroughCallID = *throughID
	}
	var historyCount int
	countQuery := `SELECT count(*) FROM agent_messages m JOIN agent_calls c USING (conversation_id)
WHERE c.conversation_id = $1::uuid AND (c.created_at, c.call_id) < ($2, $3::uuid)
  AND ($4::timestamptz IS NULL OR (c.created_at, c.call_id) > ($4, $5::uuid))`
	if err := r.pool.QueryRow(ctx, countQuery, window.ConversationID, targetCreatedAt, callID, throughAt, throughID).Scan(&historyCount); err != nil {
		return domain.ContextWindow{}, err
	}
	if recentLimit == 0 || historyCount == 0 {
		window.Recent = []domain.Message{}
		return window, nil
	}
	recentQuery := `SELECT m.message_id, c.call_id::text, c.created_at, m.role, m.content
FROM agent_messages m JOIN agent_calls c USING (conversation_id)
WHERE c.conversation_id = $1::uuid AND (c.created_at, c.call_id) < ($2, $3::uuid)
  AND ($4::timestamptz IS NULL OR (c.created_at, c.call_id) > ($4, $5::uuid))
ORDER BY c.created_at DESC, c.call_id DESC,
  CASE m.role WHEN 'assistant' THEN 2 ELSE 1 END DESC, m.message_id DESC
LIMIT $6`
	rows, err := r.pool.Query(ctx, recentQuery, window.ConversationID, targetCreatedAt, callID, throughAt, throughID, recentLimit)
	if err != nil {
		return domain.ContextWindow{}, err
	}
	recentRows := make([]contextHistoryRow, 0, recentLimit)
	for rows.Next() {
		var row contextHistoryRow
		if err := rows.Scan(&row.MessageID, &row.CallID, &row.CreatedAt, &row.Role, &row.Content); err != nil {
			rows.Close()
			return domain.ContextWindow{}, err
		}
		recentRows = append(recentRows, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.ContextWindow{}, err
	}
	rows.Close()
	if len(recentRows) == 0 {
		window.Recent = []domain.Message{}
		return window, nil
	}
	oldestRecent := recentRows[len(recentRows)-1]
	recentBoundary := contextCursor{CallID: oldestRecent.CallID, CreatedAt: oldestRecent.CreatedAt}
	recentMessages, err := r.messagesInContextRange(ctx, window.ConversationID, targetCreatedAt, callID, throughAt, throughID, &recentBoundary, nil)
	if err != nil {
		return domain.ContextWindow{}, err
	}
	window.Recent = make([]domain.Message, 0, len(recentMessages))
	for _, message := range recentMessages {
		window.Recent = append(window.Recent, message.Message)
	}
	if historyCount <= compressAfter {
		return window, nil
	}
	var pendingCount int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM agent_messages m JOIN agent_calls c USING (conversation_id)
WHERE c.conversation_id = $1::uuid AND (c.created_at, c.call_id) < ($2, $3::uuid)
  AND (c.created_at, c.call_id) < ($4, $5::uuid)
  AND ($6::timestamptz IS NULL OR (c.created_at, c.call_id) > ($6, $7::uuid))`,
		window.ConversationID, targetCreatedAt, callID, recentBoundary.CreatedAt, recentBoundary.CallID, throughAt, throughID).Scan(&pendingCount); err != nil {
		return domain.ContextWindow{}, err
	}
	if pendingCount == 0 {
		return window, nil
	}
	groupRows, err := r.pool.Query(ctx, `SELECT c.call_id::text, c.created_at, count(m.message_id)
FROM agent_messages m JOIN agent_calls c USING (conversation_id)
WHERE c.conversation_id = $1::uuid AND (c.created_at, c.call_id) < ($2, $3::uuid)
  AND (c.created_at, c.call_id) < ($4, $5::uuid)
  AND ($6::timestamptz IS NULL OR (c.created_at, c.call_id) > ($6, $7::uuid))
GROUP BY c.call_id, c.created_at ORDER BY c.created_at, c.call_id`,
		window.ConversationID, targetCreatedAt, callID, recentBoundary.CreatedAt, recentBoundary.CallID, throughAt, throughID)
	if err != nil {
		return domain.ContextWindow{}, err
	}
	selected := 0
	var pendingThrough contextCursor
	for groupRows.Next() {
		var cursor contextCursor
		var messages int
		if err := groupRows.Scan(&cursor.CallID, &cursor.CreatedAt, &messages); err != nil {
			groupRows.Close()
			return domain.ContextWindow{}, err
		}
		if selected > 0 && selected+messages > batchMessages {
			break
		}
		selected += messages
		pendingThrough = cursor
		if selected >= batchMessages {
			break
		}
	}
	if err := groupRows.Err(); err != nil {
		groupRows.Close()
		return domain.ContextWindow{}, err
	}
	groupRows.Close()
	if pendingThrough.CallID == "" {
		return window, nil
	}
	window.PendingThroughCallID = pendingThrough.CallID
	window.PendingThroughCreatedAt = pendingThrough.CreatedAt
	pending, err := r.messagesInContextRange(ctx, window.ConversationID, targetCreatedAt, callID, throughAt, throughID, nil, &pendingThrough)
	if err != nil {
		return domain.ContextWindow{}, err
	}
	window.Pending = pending
	return window, nil
}

func (r *Calls) messagesInContextRange(ctx context.Context, conversationID string, targetCreatedAt time.Time, targetCallID string, afterAt *time.Time, afterID *string, fromInclusive, toInclusive *contextCursor) ([]domain.ContextMessage, error) {
	query := `SELECT c.call_id::text, c.created_at, m.role, m.content FROM agent_messages m JOIN agent_calls c USING (conversation_id)
WHERE c.conversation_id = $1::uuid AND (c.created_at, c.call_id) < ($2, $3::uuid)
  AND ($4::timestamptz IS NULL OR (c.created_at, c.call_id) > ($4, $5::uuid))`
	args := []any{conversationID, targetCreatedAt, targetCallID, afterAt, afterID}
	if fromInclusive != nil {
		query += fmt.Sprintf(" AND (c.created_at, c.call_id) >= ($%d, $%d::uuid)", len(args)+1, len(args)+2)
		args = append(args, fromInclusive.CreatedAt, fromInclusive.CallID)
	}
	if toInclusive != nil {
		query += fmt.Sprintf(" AND (c.created_at, c.call_id) <= ($%d, $%d::uuid)", len(args)+1, len(args)+2)
		args = append(args, toInclusive.CreatedAt, toInclusive.CallID)
	}
	query += ` ORDER BY c.created_at, c.call_id,
CASE m.role WHEN 'assistant' THEN 1 ELSE 0 END, m.message_id`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]domain.ContextMessage, 0)
	for rows.Next() {
		var message domain.ContextMessage
		if err := rows.Scan(&message.CallID, &message.CreatedAt, &message.Role, &message.Content); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (r *Calls) SaveContextSummary(ctx context.Context, conversationID, expectedCallID string, expectedCreatedAt time.Time, throughCallID string, throughCreatedAt time.Time, summary string) (saved bool, resultErr error) {
	if conversationID == "" || throughCallID == "" || summary == "" {
		return false, domain.ErrConflict
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { resultErr = errors.Join(resultErr, rollbackTransaction(ctx, tx)) }()
	if _, err := tx.Exec(ctx, `INSERT INTO agent_context_summaries
(conversation_id, agent_type, summary, through_call_created_at, through_call_id)
SELECT conversation_id, agent_type, '', NULL, NULL FROM agent_conversations WHERE conversation_id = $1::uuid
ON CONFLICT (conversation_id) DO NOTHING`, conversationID); err != nil {
		return false, err
	}
	var expectedAt any
	var expectedID any
	if expectedCallID != "" {
		expectedAt, expectedID = expectedCreatedAt, expectedCallID
	}
	var updated string
	err = tx.QueryRow(ctx, `UPDATE agent_context_summaries
SET summary = $6, through_call_created_at = $5, through_call_id = $4::uuid, updated_at = now()
WHERE conversation_id = $1::uuid
  AND through_call_created_at IS NOT DISTINCT FROM $2::timestamptz
  AND through_call_id IS NOT DISTINCT FROM $3::uuid
RETURNING conversation_id::text`, conversationID, expectedAt, expectedID, throughCallID, throughCreatedAt, summary).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Calls) ClaimQueued(ctx context.Context, limit int) ([]domain.Call, error) {
	rows, err := r.pool.Query(ctx, `WITH candidates AS (
 SELECT c.call_id FROM agent_calls c
 WHERE c.status = 'queued'
   AND c.call_id = (SELECT q.call_id FROM agent_calls q WHERE q.conversation_id = c.conversation_id
     AND q.status = 'queued' ORDER BY q.created_at LIMIT 1)
   AND NOT EXISTS (SELECT 1 FROM agent_calls active WHERE active.conversation_id = c.conversation_id
     AND active.status IN ('submitting','started','running','processing'))
 ORDER BY c.created_at LIMIT $1 FOR UPDATE OF c SKIP LOCKED
), claimed AS (
 UPDATE agent_calls c SET status = 'submitting', updated_at = now()
 FROM candidates x WHERE c.call_id = x.call_id
 RETURNING c.call_id, c.agent_type, c.request_json, c.status, c.hermes_run_id, c.output, c.error, c.retry_count, c.created_at, c.updated_at, c.conversation_id
)
SELECT c.call_id::text, c.agent_type, v.scope_id, v.conversation_id::text, c.request_json, c.status, c.hermes_run_id,
 c.output, c.error, c.retry_count, c.created_at, c.updated_at
FROM claimed c JOIN agent_conversations v USING (conversation_id) ORDER BY c.created_at`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var calls []domain.Call
	for rows.Next() {
		var call domain.Call
		if err := rows.Scan(&call.ID, &call.Agent, &call.ScopeID, &call.ConversationID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.RetryCount, &call.CreatedAt, &call.UpdatedAt); err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}
	return calls, rows.Err()
}

func (r *Calls) SetRun(ctx context.Context, callID, runID string) error {
	result, err := r.pool.Exec(ctx, `UPDATE agent_calls SET hermes_run_id = $2, status = 'started', updated_at = now()
WHERE call_id = $1 AND status = 'submitting'`, callID, runID)
	if err == nil && result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

func (r *Calls) ListActive(ctx context.Context) ([]domain.Call, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.call_id::text, c.agent_type, v.scope_id, v.conversation_id::text, c.request_json, c.status,
c.hermes_run_id, c.output, c.error, c.retry_count, c.created_at, c.updated_at FROM agent_calls c
JOIN agent_conversations v USING (conversation_id) WHERE c.status IN ('started','running') AND c.hermes_run_id IS NOT NULL
ORDER BY c.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var calls []domain.Call
	for rows.Next() {
		var call domain.Call
		if err := rows.Scan(&call.ID, &call.Agent, &call.ScopeID, &call.ConversationID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.RetryCount, &call.CreatedAt, &call.UpdatedAt); err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}
	return calls, rows.Err()
}

func (r *Calls) SetStatus(ctx context.Context, callID string, status domain.CallStatus, message string) error {
	var errorValue any
	if message != "" {
		errorValue = message
	}
	_, err := r.pool.Exec(ctx, `UPDATE agent_calls SET status = $2, error = $3, updated_at = now()
WHERE call_id = $1 AND status IN ('submitting','started','running')`, callID, string(status), errorValue)
	return err
}

func (r *Calls) Complete(ctx context.Context, callID, output string) error {
	_, _, err := r.complete(ctx, callID, output, nil, "")
	return err
}

func (r *Calls) CompleteNews(ctx context.Context, callID, output, contentHash string) error {
	if contentHash == "" {
		return domain.ErrConflict
	}
	_, _, err := r.complete(ctx, callID, output, nil, contentHash)
	return err
}

func (r *Calls) CompleteValera(ctx context.Context, callID, output, companyID string, expectedRevision int64) (domain.CompanyProfileSyncState, bool, error) {
	if expectedRevision < 1 {
		return domain.CompanyProfileSyncState{}, false, domain.ErrConflict
	}
	return r.complete(ctx, callID, output, &valeraProfileCheck{companyID: companyID, revision: expectedRevision}, "")
}

type valeraProfileCheck struct {
	companyID string
	revision  int64
}

func (r *Calls) complete(ctx context.Context, callID, output string, profileCheck *valeraProfileCheck, newsTextHash string) (profileState domain.CompanyProfileSyncState, completed bool, resultErr error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.CompanyProfileSyncState{}, false, err
	}
	defer func() { resultErr = errors.Join(resultErr, rollbackTransaction(ctx, tx)) }()
	if profileCheck != nil {
		err = tx.QueryRow(ctx, `SELECT profile_revision, profile_synced FROM company_profile_state
WHERE company_id = $1::uuid FOR UPDATE`, profileCheck.companyID).Scan(&profileState.Revision, &profileState.Synced)
		if errors.Is(err, pgx.ErrNoRows) {
			return profileState, false, nil
		}
		if err != nil {
			return profileState, false, err
		}
		if !profileState.Synced || profileState.Revision != profileCheck.revision {
			return profileState, false, nil
		}
	}
	var conversationID, agentType string
	var inputJSON []byte
	query := `UPDATE agent_calls SET status = 'processing', updated_at = now()
WHERE call_id = $1 AND status IN ('started','running')`
	if profileCheck != nil {
		query += ` AND agent_type = 'valera'`
	}
	query += ` RETURNING conversation_id::text, agent_type, request_json`
	err = tx.QueryRow(ctx, query, callID).Scan(&conversationID, &agentType, &inputJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return profileState, false, nil
	}
	if err != nil {
		return profileState, false, err
	}
	if newsTextHash != "" {
		if agentType != string(domain.AgentSerezha) {
			return profileState, false, domain.ErrConflict
		}
		sanitizedInput, err := sanitizedNewsInput(inputJSON, newsTextHash)
		if err != nil {
			return profileState, false, err
		}
		if _, err = tx.Exec(ctx, `UPDATE agent_calls SET request_json = $2::jsonb WHERE call_id = $1`, callID, sanitizedInput); err != nil {
			return profileState, false, err
		}
		if _, err = tx.Exec(ctx, `UPDATE agent_messages SET content = $2 WHERE call_id = $1 AND role = 'user'`, callID, string(sanitizedInput)); err != nil {
			return profileState, false, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO agent_messages (conversation_id, call_id, role, content) VALUES ($1, $2, 'assistant', $3)`, conversationID, callID, output); err != nil {
		return profileState, false, err
	}
	if agentType == string(domain.AgentKonstantin) {
		if err := insertKonstantinEvent(ctx, tx, callID, conversationID, inputJSON, output); err != nil {
			return profileState, false, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_calls SET status = 'completed', output = $2, error = NULL, updated_at = now() WHERE call_id = $1`, callID, output); err != nil {
		return profileState, false, err
	}
	_, err = tx.Exec(ctx, `UPDATE agent_conversations SET updated_at = now() WHERE conversation_id = $1`, conversationID)
	if err != nil {
		return profileState, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return profileState, false, err
	}
	return profileState, true, nil
}

func sanitizedNewsInput(raw []byte, expectedHash string) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, domain.ErrConflict
	}
	if encodedText, ok := fields["news_text"]; ok {
		var text string
		if err := json.Unmarshal(encodedText, &text); err != nil {
			return nil, domain.ErrConflict
		}
		digest := sha256.Sum256([]byte(text))
		if hex.EncodeToString(digest[:]) != expectedHash {
			return nil, domain.ErrConflict
		}
		delete(fields, "news_text")
	} else if encodedHash, ok := fields["news_text_sha256"]; ok {
		var actualHash string
		if err := json.Unmarshal(encodedHash, &actualHash); err != nil || actualHash != expectedHash {
			return nil, domain.ErrConflict
		}
	} else {
		return nil, domain.ErrConflict
	}
	encodedHash, err := json.Marshal(expectedHash)
	if err != nil {
		return nil, err
	}
	fields["news_text_sha256"] = encodedHash
	return json.Marshal(fields)
}

func (r *Calls) RetryProfileSave(ctx context.Context, callID, companyID string, expectedRevision int64, maxRetries int) (bool, error) {
	result, err := r.pool.Exec(ctx, `UPDATE agent_calls c
SET status = 'queued', retry_count = retry_count + 1, hermes_run_id = NULL, output = NULL,
    error = 'profile_sync_retry', updated_at = now()
WHERE c.call_id = $1 AND c.agent_type = 'valera' AND c.status IN ('started','running') AND c.retry_count < $4
  AND EXISTS (SELECT 1 FROM agent_conversations v WHERE v.conversation_id = c.conversation_id AND v.scope_id = $2)
  AND NOT EXISTS (SELECT 1 FROM company_profile_state s WHERE s.company_id = $2::uuid
    AND (s.profile_revision > $3 OR (s.profile_revision = $3 AND s.profile_synced)))`,
		callID, companyID, expectedRevision, maxRetries)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

var evidenceSourcePattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type konstantinInput struct {
	RequestID       string `json:"request_id"`
	CompanyID       string `json:"company_id"`
	AnalysisID      string `json:"analysis_id"`
	RequestKind     string `json:"request_kind"`
	ProfileRevision int64  `json:"profile_revision"`
	SourceRevision  int64  `json:"source_revision"`
}

type assistantOutput struct {
	Text     string                      `json:"text"`
	Answer   string                      `json:"answer"`
	Evidence []domain.ConsultantEvidence `json:"evidence"`
}

func insertKonstantinEvent(ctx context.Context, tx pgx.Tx, callID, conversationID string, inputJSON []byte, output string) error {
	var input konstantinInput
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return err
	}
	var result assistantOutput
	if json.Unmarshal([]byte(strings.TrimSpace(output)), &result) != nil {
		result.Text = strings.TrimSpace(output)
	}
	if result.Text == "" {
		result.Text = strings.TrimSpace(result.Answer)
	}
	if result.Text == "" {
		result.Text = strings.TrimSpace(output)
	}
	if result.Evidence == nil {
		result.Evidence = []domain.ConsultantEvidence{}
	}
	validatedEvidence := make([]domain.ConsultantEvidence, 0, len(result.Evidence))
	for _, item := range result.Evidence {
		item.SourceID = strings.ToLower(strings.TrimSpace(item.SourceID))
		if !evidenceSourcePattern.MatchString(item.SourceID) {
			continue
		}
		var materialKind string
		err := tx.QueryRow(ctx, `SELECT material_kind FROM company_news_sources WHERE company_id = $1::uuid AND source_id = $2::uuid`, input.CompanyID, item.SourceID).Scan(&materialKind)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		switch item.OriginKind {
		case "ai_inference":
		case "fact":
			if materialKind != "external_news" {
				continue
			}
		case "bank_view":
			if materialKind != "alpha_analysis" {
				continue
			}
		default:
			continue
		}
		validatedEvidence = append(validatedEvidence, item)
	}
	result.Evidence = validatedEvidence
	kind := "analysis_result"
	if input.RequestKind == "question" {
		kind = "assistant_answer"
	}
	if input.RequestID == "" {
		input.RequestID = callID
	}
	if input.AnalysisID == "" {
		input.AnalysisID = input.RequestID
	}
	evidence, err := json.Marshal(result.Evidence)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO konstantin_messages
(request_id, call_id, company_id, conversation_id, analysis_id, kind, text, evidence, profile_revision, source_revision)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8::jsonb, $9, $10)
ON CONFLICT (request_id) DO NOTHING`, input.RequestID, callID, input.CompanyID, conversationID, input.AnalysisID, kind, result.Text, string(evidence), input.ProfileRevision, input.SourceRevision)
	return err
}

func (r *Calls) Fail(ctx context.Context, callID, message string) error {
	_, err := r.pool.Exec(ctx, `UPDATE agent_calls SET status = 'failed', error = $2, updated_at = now()
WHERE call_id = $1 AND status IN ('queued','submitting')`, callID, message)
	return err
}

func (r *Calls) Recover(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, `UPDATE agent_calls SET status = 'queued', updated_at = now() WHERE status = 'submitting' AND hermes_run_id IS NULL`); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `UPDATE agent_calls SET status = 'interrupted', error = 'service_restarted_during_processing', updated_at = now() WHERE status = 'processing'`)
	return err
}

func visibleASCII(value string) bool {
	for _, b := range []byte(value) {
		if b < 0x21 || b > 0x7e {
			return false
		}
	}
	return true
}

var _ domain.CallRepository = (*Calls)(nil)
