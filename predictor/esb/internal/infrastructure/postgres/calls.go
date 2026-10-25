package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/predictboss/predictor/esb/internal/domain"
)

type Calls struct{ pool *pgxpool.Pool }

func NewCalls(pool *pgxpool.Pool) *Calls { return &Calls{pool: pool} }

func (r *Calls) Enqueue(ctx context.Context, agent domain.AgentType, scopeID string, input json.RawMessage, idempotencyKey string) (domain.Call, bool, error) {
	if len(idempotencyKey) > 255 || !visibleASCII(idempotencyKey) {
		return domain.Call{}, false, domain.ErrConflict
	}
	if idempotencyKey != "" {
		call, sameRequest, err := r.byIdempotency(ctx, idempotencyKey, input)
		if err == nil {
			if !sameRequest || call.Agent != agent || call.ScopeID != scopeID {
				return domain.Call{}, false, domain.ErrConflict
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
	defer tx.Rollback(ctx)
	var conversationID string
	err = tx.QueryRow(ctx, `INSERT INTO agent_conversations (agent_type, scope_id)
VALUES ($1, $2)
ON CONFLICT (agent_type, scope_id) DO UPDATE SET updated_at = now()
RETURNING conversation_id::text`, string(agent), scopeID).Scan(&conversationID)
	if err != nil {
		return domain.Call{}, false, err
	}
	var call domain.Call
	call.Agent, call.ScopeID, call.Input, call.Status = agent, scopeID, input, domain.CallQueued
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
	err := r.pool.QueryRow(ctx, `SELECT c.call_id::text, c.agent_type, v.scope_id, c.request_json, c.status,
c.hermes_run_id, c.output, c.error, c.created_at, c.updated_at, c.request_json = $2::jsonb
FROM agent_calls c JOIN agent_conversations v USING (conversation_id) WHERE c.idempotency_key = $1`, key, input).
		Scan(&call.ID, &call.Agent, &call.ScopeID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.CreatedAt, &call.UpdatedAt, &sameRequest)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Call{}, false, domain.ErrNotFound
	}
	return call, sameRequest, err
}

func (r *Calls) Get(ctx context.Context, id string) (domain.Call, error) {
	return r.get(ctx, id, "", false)
}

func (r *Calls) GetForAgent(ctx context.Context, id string, agent domain.AgentType) (domain.Call, error) {
	return r.get(ctx, id, agent, true)
}

func (r *Calls) get(ctx context.Context, id string, agent domain.AgentType, constrain bool) (domain.Call, error) {
	query := `SELECT c.call_id::text, c.agent_type, v.scope_id, c.request_json, c.status, c.hermes_run_id,
c.output, c.error, c.created_at, c.updated_at FROM agent_calls c JOIN agent_conversations v USING (conversation_id)
WHERE c.call_id = $1`
	if constrain {
		query += ` AND c.agent_type = $2`
	}
	var call domain.Call
	var err error
	if constrain {
		err = r.pool.QueryRow(ctx, query, id, string(agent)).Scan(&call.ID, &call.Agent, &call.ScopeID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.CreatedAt, &call.UpdatedAt)
	} else {
		err = r.pool.QueryRow(ctx, query, id).Scan(&call.ID, &call.Agent, &call.ScopeID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.CreatedAt, &call.UpdatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Call{}, domain.ErrNotFound
	}
	return call, err
}

func (r *Calls) LoadHistory(ctx context.Context, callID string, limit int) ([]domain.Message, error) {
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
 RETURNING c.call_id, c.agent_type, c.request_json, c.status, c.hermes_run_id, c.output, c.error, c.created_at, c.updated_at, c.conversation_id
)
SELECT c.call_id::text, c.agent_type, v.scope_id, c.request_json, c.status, c.hermes_run_id,
 c.output, c.error, c.created_at, c.updated_at
FROM claimed c JOIN agent_conversations v USING (conversation_id) ORDER BY c.created_at`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var calls []domain.Call
	for rows.Next() {
		var call domain.Call
		if err := rows.Scan(&call.ID, &call.Agent, &call.ScopeID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.CreatedAt, &call.UpdatedAt); err != nil {
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
	rows, err := r.pool.Query(ctx, `SELECT c.call_id::text, c.agent_type, v.scope_id, c.request_json, c.status,
c.hermes_run_id, c.output, c.error, c.created_at, c.updated_at FROM agent_calls c
JOIN agent_conversations v USING (conversation_id) WHERE c.status IN ('started','running') AND c.hermes_run_id IS NOT NULL
ORDER BY c.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var calls []domain.Call
	for rows.Next() {
		var call domain.Call
		if err := rows.Scan(&call.ID, &call.Agent, &call.ScopeID, &call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error, &call.CreatedAt, &call.UpdatedAt); err != nil {
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var conversationID string
	err = tx.QueryRow(ctx, `UPDATE agent_calls SET status = 'processing', updated_at = now()
WHERE call_id = $1 AND status IN ('started','running') RETURNING conversation_id::text`, callID).Scan(&conversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO agent_messages (conversation_id, call_id, role, content) VALUES ($1, $2, 'assistant', $3)`, conversationID, callID, output); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_calls SET status = 'completed', output = $2, error = NULL, updated_at = now() WHERE call_id = $1`, callID, output); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE agent_conversations SET updated_at = now() WHERE conversation_id = $1`, conversationID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Calls) Fail(ctx context.Context, callID, message string) error {
	_, err := r.pool.Exec(ctx, `UPDATE agent_calls SET status = 'failed', error = $2, updated_at = now()
WHERE call_id = $1 AND status IN ('queued','submitting')`, callID, message)
	return err
}

func (r *Calls) Recover(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, `ALTER TABLE agent_conversations DROP CONSTRAINT IF EXISTS agent_conversations_agent_type_check`); err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, `ALTER TABLE agent_calls DROP CONSTRAINT IF EXISTS agent_calls_agent_type_check`); err != nil {
		return err
	}
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
