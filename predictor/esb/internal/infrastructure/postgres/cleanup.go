package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

const cleanupAdvisoryLockKey int64 = 0x505245444943544f

// WithCleanupLock holds one PostgreSQL session advisory lock across a bounded
// maintenance pass. The dedicated acquired connection keeps the lock alive
// while normal pool operations perform work.
func (r *Calls) WithCleanupLock(ctx context.Context, work func(context.Context) error) (locked bool, resultErr error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	if work == nil {
		conn.Release()
		return false, errors.New("cleanup lock work is required")
	}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, cleanupAdvisoryLockKey).Scan(&acquired); err != nil {
		return false, discardPoolConnection(ctx, conn, fmt.Errorf("acquire PostgreSQL cleanup lock: %w", err))
	}
	if !acquired {
		conn.Release()
		return false, nil
	}
	defer func() {
		resultErr = errors.Join(resultErr, releaseAdvisoryLock(ctx, conn,
			`SELECT pg_advisory_unlock($1)`, cleanupAdvisoryLockKey))
	}()
	return true, work(ctx)
}

func (r *Calls) RecoverStaleSubmissions(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	var count int
	err := r.pool.QueryRow(ctx, `WITH stale AS (
 SELECT call_id FROM agent_calls
	 WHERE status = 'submitting' AND hermes_run_id IS NULL AND launch_started_at IS NULL
	   AND cancel_requested_at IS NULL AND execution_deadline_at > now() AND updated_at < $1
 ORDER BY updated_at, call_id LIMIT $2 FOR UPDATE SKIP LOCKED
), recovered AS (
 UPDATE agent_calls c SET status = 'queued', error = 'stale_submission_recovered', updated_at = now()
 FROM stale s WHERE c.call_id = s.call_id RETURNING c.call_id
) SELECT count(*) FROM recovered`, before, limit).Scan(&count)
	return count, err
}

func (r *Calls) ListStaleActive(ctx context.Context, before time.Time, limit int) ([]domain.Call, error) {
	if limit <= 0 {
		return []domain.Call{}, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+executionCallColumns+`
FROM agent_calls c JOIN agent_conversations v USING (conversation_id)
WHERE c.status IN ('started','running') AND c.hermes_run_id IS NOT NULL AND c.updated_at < $1
ORDER BY c.updated_at, c.call_id LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	calls := make([]domain.Call, 0)
	for rows.Next() {
		call, err := scanExecutionCall(rows)
		if err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}
	return calls, rows.Err()
}

func (r *Calls) RetryStaleActive(ctx context.Context, call domain.Call, maxRetries int) (bool, error) {
	if maxRetries <= 0 || call.RetryCount >= maxRetries || call.HermesRunID == nil {
		return false, nil
	}
	result, err := r.pool.Exec(ctx, `UPDATE agent_calls SET status = 'queued', retry_count = retry_count + 1,
    hermes_run_id = NULL, runtime_instance_id = NULL, launch_started_at = NULL, output = NULL,
    error = 'cleanup_retry', updated_at = now()
WHERE call_id = $1 AND status IN ('started','running') AND updated_at = $2
 AND hermes_run_id = $3 AND retry_count < $4
 AND cancel_requested_at IS NULL AND execution_deadline_at > now()`, call.ID, call.UpdatedAt, *call.HermesRunID, maxRetries)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

func (r *Calls) PruneCallPayloads(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	var count int
	err := r.pool.QueryRow(ctx, `WITH candidates AS (
 SELECT call_id FROM agent_calls
 WHERE created_at < $1 AND status IN ('completed','failed','cancelled','interrupted')
  AND (request_json <> '{}'::jsonb OR output IS NOT NULL OR error IS NOT NULL)
  AND (status = 'completed' OR agent_type <> 'serezha')
 ORDER BY created_at, call_id LIMIT $2 FOR UPDATE SKIP LOCKED
), pruned AS (
 UPDATE agent_calls c SET request_json = '{}'::jsonb, output = NULL, error = NULL
 FROM candidates x WHERE c.call_id = x.call_id RETURNING c.call_id
) SELECT count(*) FROM pruned`, before, limit).Scan(&count)
	return count, err
}

func (r *Calls) PruneCallRecords(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	var deleted, compacted int
	if err := r.pool.QueryRow(ctx, `WITH candidates AS (
 SELECT c.call_id FROM agent_calls c
 WHERE c.created_at < $1 AND c.status IN ('completed','failed','cancelled','interrupted')
  AND c.idempotency_key IS NULL
  AND NOT EXISTS (SELECT 1 FROM agent_messages m WHERE m.call_id = c.call_id)
  AND NOT EXISTS (SELECT 1 FROM konstantin_messages k WHERE k.call_id = c.call_id)
  AND NOT EXISTS (SELECT 1 FROM agent_context_summaries s WHERE s.through_call_id = c.call_id)
  AND NOT (c.agent_type = 'serezha' AND c.request_json ? 'news_text')
 ORDER BY c.created_at, c.call_id LIMIT $2 FOR UPDATE SKIP LOCKED
), removed AS (
 DELETE FROM agent_calls c USING candidates x WHERE c.call_id = x.call_id RETURNING c.call_id
) SELECT count(*) FROM removed`, before, limit).Scan(&deleted); err != nil {
		return 0, err
	}
	if err := r.pool.QueryRow(ctx, `WITH candidates AS (
 SELECT call_id FROM agent_calls
 WHERE created_at < $1 AND status IN ('completed','failed','cancelled','interrupted')
  AND NOT (agent_type = 'serezha' AND request_json ? 'news_text')
  AND (request_json <> '{}'::jsonb OR output IS NOT NULL OR error IS NOT NULL OR hermes_run_id IS NOT NULL)
 ORDER BY created_at, call_id LIMIT $2 FOR UPDATE SKIP LOCKED
), compacted AS (
 UPDATE agent_calls c SET request_json = CASE WHEN c.agent_type = 'serezha' THEN
   jsonb_strip_nulls(jsonb_build_object('news_id', c.request_json->'news_id',
    'source_id', c.request_json->'source_id', 'source_url', c.request_json->'source_url',
    'material_kind', c.request_json->'material_kind', 'media_type', c.request_json->'media_type',
    'title', c.request_json->'title', 'published_at', c.request_json->'published_at',
    'news_text_sha256', c.request_json->'news_text_sha256'))
   ELSE '{}'::jsonb END, output = NULL, error = NULL, hermes_run_id = NULL
 FROM candidates x WHERE c.call_id = x.call_id RETURNING c.call_id
) SELECT count(*) FROM compacted`, before, limit).Scan(&compacted); err != nil {
		return deleted, err
	}
	return deleted + compacted, nil
}

func (r *Calls) PruneMessages(ctx context.Context, agent domain.AgentType, before time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	var removed int
	if agent == domain.AgentKonstantin {
		var count int
		err := r.pool.QueryRow(ctx, `WITH candidates AS (
 SELECT m.message_id FROM konstantin_messages m
 WHERE m.created_at < $1 ORDER BY m.created_at, m.message_id LIMIT $2 FOR UPDATE SKIP LOCKED
), deleted AS (
 DELETE FROM konstantin_messages m USING candidates c WHERE m.message_id = c.message_id RETURNING m.message_id
) SELECT count(*) FROM deleted`, before, limit).Scan(&count)
		if err != nil {
			return 0, err
		}
		removed += count
	}
	var count int
	err := r.pool.QueryRow(ctx, `WITH candidates AS (
 SELECT m.message_id FROM agent_messages m
 JOIN agent_conversations v USING (conversation_id)
 JOIN agent_calls c ON c.call_id = m.call_id
 WHERE v.agent_type = $1 AND m.created_at < $2
  AND c.status IN ('completed','failed','cancelled','interrupted')
 ORDER BY m.created_at, m.message_id LIMIT $3 FOR UPDATE OF m SKIP LOCKED
), deleted AS (
 DELETE FROM agent_messages m USING candidates c WHERE m.message_id = c.message_id RETURNING m.message_id
) SELECT count(*) FROM deleted`, string(agent), before, limit).Scan(&count)
	return removed + count, err
}

func (r *Calls) PruneContextSummaries(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	var count int
	err := r.pool.QueryRow(ctx, `WITH candidates AS (
 SELECT conversation_id FROM agent_context_summaries
 WHERE updated_at < $1 ORDER BY updated_at, conversation_id LIMIT $2 FOR UPDATE SKIP LOCKED
), deleted AS (
 DELETE FROM agent_context_summaries s USING candidates c WHERE s.conversation_id = c.conversation_id
 RETURNING s.conversation_id
) SELECT count(*) FROM deleted`, before, limit).Scan(&count)
	return count, err
}

func (r *Calls) TerminalNewsIDs(ctx context.Context, before time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT c.request_json->>'news_id'
FROM agent_calls c
WHERE c.agent_type = 'serezha' AND c.status IN ('failed','cancelled','interrupted')
 AND c.updated_at < $1 AND c.request_json ? 'news_id' AND c.request_json->>'news_id' <> ''
ORDER BY c.request_json->>'news_id' LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

var _ domain.CleanupRepository = (*Calls)(nil)
