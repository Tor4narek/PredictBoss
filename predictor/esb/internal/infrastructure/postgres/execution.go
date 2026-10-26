package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/predictboss/predictor/esb/internal/domain"
)

const executionCallColumns = `c.call_id::text, c.agent_type, v.scope_id, v.conversation_id::text,
c.request_json, c.status, c.hermes_run_id, c.output, c.error, c.retry_count,
c.created_at, c.updated_at, c.execution_deadline_at, c.launch_started_at,
c.cancel_requested_at, c.stop_attempted_at, c.stop_failed_at, COALESCE(c.stop_reason, ''),
c.event_sequence, COALESCE(c.cancel_sequence, 0), COALESCE(c.stop_failure_sequence, 0),
COALESCE(c.terminal_sequence, 0), COALESCE(c.runtime_instance_id, '')`

// CancelKonstantin atomically wins or loses the race against a queue worker or
// a terminal Hermes result. It stores only lifecycle state, never message chunks.
func (r *Calls) CancelKonstantin(ctx context.Context, companyID, conversationID, requestID string, now time.Time) (result domain.CancelResult, resultErr error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.CancelResult{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, rollbackTransaction(ctx, tx)) }()
	call, err := scanExecutionCall(tx.QueryRow(ctx, `SELECT `+executionCallColumns+`
FROM agent_calls c JOIN agent_conversations v USING (conversation_id)
WHERE c.agent_type = 'konstantin' AND c.request_json->>'request_kind' = 'question'
  AND c.request_json->>'request_id' = $1 AND v.scope_id = $2 AND v.conversation_id = $3::uuid
FOR UPDATE OF c`, requestID, companyID, conversationID))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.CancelResult{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.CancelResult{}, err
	}
	if call.Status.Terminal() {
		disposition := domain.CancelTerminal
		if call.Status == domain.CallCompleted {
			disposition = domain.CancelCompleted
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.CancelResult{}, err
		}
		return domain.CancelResult{Call: call, Disposition: disposition}, nil
	}
	if call.CancelRequestedAt != nil {
		disposition := domain.CancelAlreadyAsked
		if call.StopFailedAt != nil {
			disposition = domain.CancelFailed
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.CancelResult{}, err
		}
		return domain.CancelResult{Call: call, Disposition: disposition}, nil
	}
	if call.Status == domain.CallQueued || (call.Status == domain.CallSubmitting && call.LaunchStartedAt == nil) {
		call, err = scanExecutionCall(tx.QueryRow(ctx, `UPDATE agent_calls c
SET status = 'cancelled', error = 'assistant_cancelled', cancel_requested_at = $2,
    stop_reason = 'user', event_sequence = event_sequence + 2,
    cancel_sequence = event_sequence + 1, terminal_sequence = event_sequence + 2,
    updated_at = $2
FROM agent_conversations v
WHERE c.call_id = $1::uuid AND c.conversation_id = v.conversation_id
RETURNING `+executionCallColumns, call.ID, now.UTC()))
		if err != nil {
			return domain.CancelResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.CancelResult{}, err
		}
		return domain.CancelResult{Call: call, Disposition: domain.CancelQueued}, nil
	}
	call, err = scanExecutionCall(tx.QueryRow(ctx, `UPDATE agent_calls c
SET cancel_requested_at = $2, stop_reason = 'user',
    event_sequence = event_sequence + 1, cancel_sequence = event_sequence + 1,
    updated_at = $2
FROM agent_conversations v
WHERE c.call_id = $1::uuid AND c.conversation_id = v.conversation_id
  AND c.status IN ('submitting', 'started', 'running', 'processing')
RETURNING `+executionCallColumns, call.ID, now.UTC()))
	if err != nil {
		return domain.CancelResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.CancelResult{}, err
	}
	return domain.CancelResult{Call: call, Disposition: domain.CancelRequested}, nil
}

// ClaimHermesStop guarantees that at most one ESB replica sends a stop request.
func (r *Calls) ClaimHermesStop(ctx context.Context, callID string, now time.Time) (domain.Call, bool, error) {
	call, err := scanExecutionCall(r.pool.QueryRow(ctx, `UPDATE agent_calls c
SET stop_attempted_at = $2, updated_at = $2
FROM agent_conversations v
WHERE c.call_id = $1::uuid AND c.conversation_id = v.conversation_id
  AND c.status IN ('submitting', 'started', 'running', 'processing')
  AND c.cancel_requested_at IS NOT NULL AND c.stop_attempted_at IS NULL
  AND c.hermes_run_id IS NOT NULL
RETURNING `+executionCallColumns, callID, now.UTC()))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Call{}, false, nil
	}
	return call, err == nil, err
}

// MarkHermesStopFailed records a safe, retry-proof stop outcome for watchers.
func (r *Calls) MarkHermesStopFailed(ctx context.Context, callID, reason string, now time.Time) (domain.Call, error) {
	if reason != "stop_failed" && reason != "stop_unsupported" {
		return domain.Call{}, domain.ErrConflict
	}
	return scanExecutionCall(r.pool.QueryRow(ctx, `UPDATE agent_calls c
SET stop_failed_at = $2, error = $3, event_sequence = event_sequence + 1,
    stop_failure_sequence = event_sequence + 1, updated_at = $2
FROM agent_conversations v
WHERE c.call_id = $1::uuid AND c.conversation_id = v.conversation_id
  AND c.status IN ('submitting', 'started', 'running', 'processing')
  AND c.cancel_requested_at IS NOT NULL AND c.stop_failed_at IS NULL
RETURNING `+executionCallColumns, callID, now.UTC(), reason))
}

// RequestExpiredCalls fails expired calls that never crossed the launch boundary
// and requests one stop for each expired active run. The returned rows are read
// state; repeated polls do not rewrite the request or extend its deadline.
func (r *Calls) RequestExpiredCalls(ctx context.Context, now time.Time, limit int) (calls []domain.Call, resultErr error) {
	if limit < 1 {
		return nil, nil
	}
	now = now.UTC()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, rollbackTransaction(ctx, tx)) }()
	if _, err := tx.Exec(ctx, `WITH expired AS (
    SELECT call_id FROM agent_calls
    WHERE status IN ('queued', 'submitting') AND execution_deadline_at <= $1
      AND (status = 'queued' OR launch_started_at IS NULL)
    ORDER BY execution_deadline_at, call_id
    LIMIT $2 FOR UPDATE SKIP LOCKED
)
UPDATE agent_calls c
SET status = 'failed', error = 'execution_deadline_exceeded', updated_at = $1,
    event_sequence = event_sequence + CASE WHEN agent_type = 'konstantin' AND request_json->>'request_kind' = 'question' THEN 1 ELSE 0 END,
    terminal_sequence = CASE WHEN agent_type = 'konstantin' AND request_json->>'request_kind' = 'question' THEN event_sequence + 1 ELSE terminal_sequence END
FROM expired x WHERE c.call_id = x.call_id`, now, limit); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `WITH expired AS (
    SELECT call_id FROM agent_calls
    WHERE status IN ('submitting', 'started', 'running', 'processing')
      AND execution_deadline_at <= $1 AND cancel_requested_at IS NULL
    ORDER BY execution_deadline_at, call_id
    LIMIT $2 FOR UPDATE SKIP LOCKED
)
UPDATE agent_calls c
SET cancel_requested_at = $1, stop_reason = 'timeout', updated_at = $1
FROM expired x WHERE c.call_id = x.call_id`, now, limit); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT `+executionCallColumns+`
FROM agent_calls c JOIN agent_conversations v USING (conversation_id)
WHERE c.status IN ('submitting', 'started', 'running', 'processing')
  AND c.stop_reason = 'timeout' AND c.cancel_requested_at IS NOT NULL
  AND c.execution_deadline_at <= $1 AND c.stop_failed_at IS NULL
ORDER BY c.execution_deadline_at, c.call_id LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	calls = make([]domain.Call, 0)
	for rows.Next() {
		call, scanErr := scanExecutionCall(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		calls = append(calls, call)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return calls, nil
}

func scanExecutionCall(scanner rowScanner) (domain.Call, error) {
	var call domain.Call
	err := scanner.Scan(&call.ID, &call.Agent, &call.ScopeID, &call.ConversationID,
		&call.Input, &call.Status, &call.HermesRunID, &call.Output, &call.Error,
		&call.RetryCount, &call.CreatedAt, &call.UpdatedAt, &call.DeadlineAt,
		&call.LaunchStartedAt, &call.CancelRequestedAt, &call.StopAttemptedAt,
		&call.StopFailedAt, &call.StopReason, &call.EventSequence, &call.CancelSequence,
		&call.StopFailureSeq, &call.TerminalSequence, &call.RuntimeInstanceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Call{}, domain.ErrNotFound
	}
	return call, err
}

var _ domain.CallExecutionRepository = (*Calls)(nil)
