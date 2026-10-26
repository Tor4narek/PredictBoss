#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"
compose=(docker compose --project-name predictor-esb-e2e --file predictor/tests/compose.e2e.yml)

processed_news_id="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
unprocessed_news_id="bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
cleanup_source_id="cccccccc-cccc-4ccc-8ccc-cccccccccccc"
cleanup_company_id="dddddddd-dddd-4ddd-8ddd-dddddddddddd"
cleanup_fact_id="eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
expired_fact_id="33333333-3333-4333-8333-333333333333"
cleanup_metric_id="ffffffff-ffff-4fff-8fff-ffffffffffff"
expired_metric_id="44444444-4444-4444-8444-444444444444"
cleanup_observation_id="11111111-1111-4111-8111-111111111111"
expired_observation_id="55555555-5555-4555-8555-555555555555"
cleanup_analysis_id="22222222-2222-4222-8222-222222222222"
expired_analysis_id="66666666-6666-4666-8666-666666666666"
processed_hash="cleanup-fixture-hash"

failed_news_call="aaaaaaaa-aaaa-4aaa-8aaa-000000000001"
retry_call="aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
submission_call="aaaaaaaa-aaaa-4aaa-8aaa-000000000003"
saveliy_call="aaaaaaaa-aaaa-4aaa-8aaa-000000000004"
konstantin_call="aaaaaaaa-aaaa-4aaa-8aaa-000000000005"
unknown_active_call="aaaaaaaa-aaaa-4aaa-8aaa-000000000006"
failed_news_conversation="bbbbbbbb-bbbb-4bbb-8bbb-000000000001"
retry_conversation="bbbbbbbb-bbbb-4bbb-8bbb-000000000002"
submission_conversation="bbbbbbbb-bbbb-4bbb-8bbb-000000000003"
saveliy_conversation="bbbbbbbb-bbbb-4bbb-8bbb-000000000004"
konstantin_conversation="bbbbbbbb-bbbb-4bbb-8bbb-000000000005"
unknown_conversation="bbbbbbbb-bbbb-4bbb-8bbb-000000000006"

clickhouse_exec() {
  "${compose[@]}" exec --no-TTY clickhouse clickhouse-client --user predictor --password test-clickhouse-password --database default "$@"
}
postgres_exec() {
  "${compose[@]}" exec --no-TTY postgres psql -X -v ON_ERROR_STOP=1 -U predictor -d predictor "$@"
}
clickhouse_value() {
  clickhouse_exec --query "$1"
}
postgres_value() {
  postgres_exec -At -c "$1"
}
wait_until_zero() {
  query="$1"
  for _ in $(seq 1 120); do
    value="$(clickhouse_value "$query" 2>/dev/null || true)"
    if [[ "$value" == "0" ]]; then
      return 0
    fi
    sleep 0.25
  done
  echo "timed out waiting for ClickHouse cleanup" >&2
  return 1
}
wait_until_value() {
  query="$1"
  expected="$2"
  for _ in $(seq 1 120); do
    value="$(clickhouse_value "$query" 2>/dev/null || true)"
    if [[ "$value" == "$expected" ]]; then
      return 0
    fi
    sleep 0.25
  done
  echo "timed out waiting for expected ClickHouse cleanup state: $expected" >&2
  return 1
}

"${compose[@]}" stop esb esb-secondary

clickhouse_exec --multiquery <<SQL
INSERT INTO default.news (news_id, source_id, source_url, inserted_at, material_kind, media_type, title, published_at)
SELECT '${processed_news_id}', '${cleanup_source_id}', 'https://example.test/cleanup', now64(3) - INTERVAL 2 DAY,
       'external_news', 'text/html', 'cleanup fixture', now64(3) - INTERVAL 2 DAY;
INSERT INTO default.news_content (news_id, content_hash, news_text, inserted_at)
SELECT '${processed_news_id}', '${processed_hash}', 'processed fixture text', now64(3) - INTERVAL 2 DAY;
INSERT INTO default.news_processing (news_id, source_id, content_hash, fact_count, processed_at)
SELECT '${processed_news_id}', '${cleanup_source_id}', '${processed_hash}', 1, now64(3) - INTERVAL 2 DAY;
INSERT INTO default.facts (fact_id, news_id, source_id, source_url, fact_text, category, entities, evidence, occurred_at, confidence, inserted_at)
SELECT toUUID('${cleanup_fact_id}'), '${processed_news_id}', '${cleanup_source_id}', 'https://example.test/cleanup',
       'cleanup fixture fact', 'market', ['fixture'], 'fixture evidence', toNullable(now64(3) - INTERVAL 2 DAY),
       toFloat32(0.9), now64(3);
INSERT INTO default.facts (fact_id, news_id, source_id, source_url, fact_text, category, entities, evidence, occurred_at, confidence, inserted_at)
SELECT toUUID('${expired_fact_id}'), 'expired-fact-news', '${cleanup_source_id}', 'https://example.test/expired-fact',
       'expired unreferenced fact', 'market', ['fixture'], 'fixture evidence', NULL,
       toFloat32(0.9), now64(3) - INTERVAL 2 DAY;
INSERT INTO default.fact_company (company_id, fact_id, relevance, confidence, inserted_at)
VALUES ('${cleanup_company_id}', toUUID('${cleanup_fact_id}'), 'high', toFloat32(0.9), now64(3)),
       ('${cleanup_company_id}', toUUID('${expired_fact_id}'), 'medium', toFloat32(0.8), now64(3) - INTERVAL 2 DAY);
INSERT INTO default.metric_observations (observation_id, company_id, metric_id, news_id, value, probability, source_fact_ids, inserted_at)
SELECT toUUID('${cleanup_observation_id}'), '${cleanup_company_id}', '${cleanup_metric_id}', '${processed_news_id}',
       toNullable(toFloat64(42)), toNullable(toFloat64(0.8)), [toUUID('${cleanup_fact_id}')], now64(3);
INSERT INTO default.metric_observations (observation_id, company_id, metric_id, news_id, value, probability, source_fact_ids, inserted_at)
SELECT toUUID('${expired_observation_id}'), '${cleanup_company_id}', '${expired_metric_id}', 'expired-observation-news',
       toNullable(toFloat64(12)), toNullable(toFloat64(0.4)), [], now64(3) - INTERVAL 2 DAY;
INSERT INTO default.metric_totals (company_id, metric_id, period_from, period_to, total_value, total_probability, fact_count, probability_count, links_json, generated_at)
SELECT '${cleanup_company_id}', '${cleanup_metric_id}', now64(3) - INTERVAL 7 DAY, now64(3) - INTERVAL 1 DAY,
       toNullable(toFloat64(42)), toNullable(toFloat64(0.8)), toUInt32(1), toUInt32(1), '{}', now64(3);
INSERT INTO default.metric_totals (company_id, metric_id, period_from, period_to, total_value, total_probability, fact_count, probability_count, links_json, generated_at)
SELECT '${cleanup_company_id}', '${expired_metric_id}', now64(3) - INTERVAL 7 DAY, now64(3) - INTERVAL 1 DAY,
       toNullable(toFloat64(12)), toNullable(toFloat64(0.4)), toUInt32(1), toUInt32(1), '{}', now64(3) - INTERVAL 2 DAY;
INSERT INTO default.company_analyses (analysis_id, company_id, period_from, period_to, probability, metric_ids, analysis_text, generated_at)
SELECT toUUID('${cleanup_analysis_id}'), '${cleanup_company_id}', toDate(now() - INTERVAL 7 DAY), toDate(now() - INTERVAL 1 DAY),
       toFloat64(0.8), ['${cleanup_metric_id}'], 'retained cleanup fixture analysis', now64(3);
INSERT INTO default.company_analyses (analysis_id, company_id, period_from, period_to, probability, metric_ids, analysis_text, generated_at)
SELECT toUUID('${expired_analysis_id}'), '${cleanup_company_id}', toDate(now() - INTERVAL 30 DAY), toDate(now() - INTERVAL 20 DAY),
       toFloat64(0.4), ['${expired_metric_id}'], 'expired cleanup fixture analysis', now64(3) - INTERVAL 2 DAY;
INSERT INTO default.news_content (news_id, content_hash, news_text, inserted_at)
SELECT '${unprocessed_news_id}', 'unprocessed-fixture-hash', 'unprocessed fixture text', now64(3) - INTERVAL 2 DAY;
SQL

"${compose[@]}" exec --no-TTY mock-runtime python -c \
  "import json,urllib.request; r=urllib.request.Request('http://127.0.0.1:8090/test/runs',data=json.dumps({'run_id':'cleanup-failed-run','status':'failed','output':'{}'}).encode(),headers={'Content-Type':'application/json'}); urllib.request.urlopen(r).read()"

postgres_exec <<SQL
INSERT INTO agent_conversations (conversation_id, agent_type, scope_id, created_at, updated_at) VALUES
('${failed_news_conversation}', 'serezha', '${unprocessed_news_id}', now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${retry_conversation}', 'kesha', '${cleanup_company_id}', now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${submission_conversation}', 'kesha', 'cleanup-submission-company', now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${saveliy_conversation}', 'saveliy', '${cleanup_company_id}', now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${konstantin_conversation}', 'konstantin', '${cleanup_company_id}', now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${unknown_conversation}', 'kesha', 'cleanup-unknown-company', now() - INTERVAL '2 days', now() - INTERVAL '2 days');
INSERT INTO agent_calls (call_id, conversation_id, agent_type, request_json, hermes_run_id, idempotency_key,
                         status, retry_count, execution_deadline_at, launch_started_at, created_at, updated_at) VALUES
('${failed_news_call}', '${failed_news_conversation}', 'serezha', '{"news_id":"${unprocessed_news_id}","source_id":"${cleanup_source_id}","news_text":"unprocessed fixture text"}'::jsonb,
 NULL, 'cleanup-news-idempotency', 'failed', 0, now() + INTERVAL '1 day', NULL, now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${retry_call}', '${retry_conversation}', 'kesha', '{"company_id":"${cleanup_company_id}"}'::jsonb,
 'cleanup-failed-run', 'cleanup-retry-idempotency', 'running', 0, now() + INTERVAL '1 day', now() - INTERVAL '2 days', now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${submission_call}', '${submission_conversation}', 'kesha', '{"company_id":"${cleanup_company_id}"}'::jsonb,
 NULL, 'cleanup-submission-idempotency', 'submitting', 0, now() + INTERVAL '1 day', NULL, now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${saveliy_call}', '${saveliy_conversation}', 'saveliy', '{"company_id":"${cleanup_company_id}"}'::jsonb,
 NULL, NULL, 'completed', 0, now() + INTERVAL '1 day', NULL, now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${konstantin_call}', '${konstantin_conversation}', 'konstantin', '{"company_id":"${cleanup_company_id}"}'::jsonb,
 NULL, 'cleanup-konstantin-idempotency', 'completed', 0, now() + INTERVAL '1 day', NULL, now() - INTERVAL '2 days', now() - INTERVAL '2 days'),
('${unknown_active_call}', '${unknown_conversation}', 'kesha', '{"company_id":"${cleanup_company_id}"}'::jsonb,
 'missing-hermes-run', NULL, 'running', 0, now() + INTERVAL '1 day', now() - INTERVAL '2 days', now() - INTERVAL '2 days', now() - INTERVAL '2 days');
INSERT INTO agent_messages (conversation_id, call_id, role, content, created_at) VALUES
('${failed_news_conversation}', '${failed_news_call}', 'user', '{"news_id":"${unprocessed_news_id}","news_text":"unprocessed fixture text"}', now() - INTERVAL '2 days'),
('${saveliy_conversation}', '${saveliy_call}', 'user', 'expired message fixture', now() - INTERVAL '2 days'),
('${konstantin_conversation}', '${konstantin_call}', 'user', 'permanent chat history fixture', now() - INTERVAL '2 days');
INSERT INTO konstantin_messages (request_id, call_id, company_id, conversation_id, analysis_id, kind, text, evidence, created_at)
VALUES ('cccccccc-cccc-4ccc-8ccc-000000000001', '${konstantin_call}', '${cleanup_company_id}', '${konstantin_conversation}',
        'cccccccc-cccc-4ccc-8ccc-000000000002', 'assistant_answer', 'permanent consultant history fixture', '[]'::jsonb, now() - INTERVAL '2 days');
INSERT INTO agent_context_summaries (conversation_id, agent_type, summary, through_call_created_at, through_call_id, updated_at)
VALUES ('${konstantin_conversation}', 'konstantin', 'expired summary fixture', now() - INTERVAL '2 days', '${konstantin_call}', now() - INTERVAL '2 days');
SQL

E2E_CLEANUP_ENABLED=true \
E2E_CLEANUP_STATUS_INTERVAL=1h \
E2E_CLEANUP_RETENTION_INTERVAL=100ms \
E2E_CLEANUP_BATCH_SIZE=1000 \
E2E_CLEANUP_QUEUED_STALE_AFTER=1s \
E2E_CLEANUP_RUNNING_STALE_AFTER=1s \
E2E_CLEANUP_ACTIVE_MAX_RETRIES=1 \
E2E_CLEANUP_CALL_PAYLOAD_RETENTION=1ns \
E2E_CLEANUP_CALL_RECORD_RETENTION=1ns \
E2E_CLEANUP_CONTEXT_SUMMARY_RETENTION=1ns \
E2E_CLEANUP_MESSAGES_RETENTION_SAVELIY=1ns \
E2E_CLEANUP_PROCESSED_NEWS_TEXT_RETENTION=1ns \
E2E_CLEANUP_UNPROCESSED_NEWS_TEXT_RETENTION=1ns \
E2E_CLEANUP_NEWS_METADATA_RETENTION=1ns \
E2E_CLEANUP_NEWS_MARKER_RETENTION=1ns \
E2E_CLEANUP_FACTS_RETENTION=1ns \
E2E_CLEANUP_FACT_COMPANY_RETENTION=1h \
E2E_CLEANUP_METRIC_OBSERVATIONS_RETENTION=1h \
E2E_CLEANUP_METRIC_TOTALS_RETENTION=1h \
E2E_CLEANUP_COMPANY_ANALYSES_RETENTION=1h \
"${compose[@]}" up --detach --force-recreate --wait esb

wait_until_zero "SELECT count() FROM default.news_content FINAL WHERE news_id IN ('${processed_news_id}','${unprocessed_news_id}')"
wait_until_value "SELECT count() FROM default.news FINAL WHERE news_id = '${processed_news_id}'" 1
wait_until_value "SELECT count() FROM default.news_processing FINAL WHERE news_id = '${processed_news_id}'" 1
wait_until_value "SELECT count() FROM default.facts FINAL WHERE fact_id = toUUID('${cleanup_fact_id}')" 1
wait_until_zero "SELECT count() FROM default.facts FINAL WHERE fact_id = toUUID('${expired_fact_id}')"
wait_until_value "SELECT count() FROM default.fact_company FINAL WHERE fact_id = toUUID('${cleanup_fact_id}')" 1
wait_until_zero "SELECT count() FROM default.fact_company FINAL WHERE fact_id = toUUID('${expired_fact_id}')"
wait_until_value "SELECT count() FROM default.metric_observations FINAL WHERE observation_id = toUUID('${cleanup_observation_id}')" 1
wait_until_zero "SELECT count() FROM default.metric_observations FINAL WHERE observation_id = toUUID('${expired_observation_id}')"
wait_until_value "SELECT count() FROM default.metric_totals FINAL WHERE company_id = '${cleanup_company_id}' AND metric_id = '${cleanup_metric_id}'" 1
wait_until_zero "SELECT count() FROM default.metric_totals FINAL WHERE company_id = '${cleanup_company_id}' AND metric_id = '${expired_metric_id}'"
wait_until_value "SELECT count() FROM default.company_analyses FINAL WHERE analysis_id = toUUID('${cleanup_analysis_id}')" 1
wait_until_zero "SELECT count() FROM default.company_analyses FINAL WHERE analysis_id = toUUID('${expired_analysis_id}')"

retry_state=""
submission_state=""
for _ in $(seq 1 120); do
  retry_state="$(postgres_value "SELECT status || ':' || retry_count FROM agent_calls WHERE call_id = '${retry_call}'" 2>/dev/null || true)"
  submission_state="$(postgres_value "SELECT status FROM agent_calls WHERE call_id = '${submission_call}'" 2>/dev/null || true)"
  if [[ "$retry_state" == "completed:1" && "$submission_state" == "completed" ]]; then
    break
  fi
  sleep 0.25
done
if [[ "$retry_state" != "completed:1" ]]; then
  call_state="$(postgres_value "SELECT status || ':' || retry_count || ':' || COALESCE(runtime_instance_id, '') || ':' || COALESCE(hermes_run_id, '') FROM agent_calls WHERE call_id = '${retry_call}'")"
  echo "stale Hermes failure was not retried once (observed=$retry_state call_state=$call_state)" >&2
  exit 1
fi
[[ "$submission_state" == "completed" ]] || { echo "stale submission without a run ID was not recovered" >&2; exit 1; }
[[ "$(postgres_value "SELECT status FROM agent_calls WHERE call_id = '${unknown_active_call}'")" == "running" ]] || { echo "unknown active Hermes status was modified" >&2; exit 1; }
[[ "$(postgres_value "SELECT count(*) FROM agent_calls WHERE call_id = '${saveliy_call}'")" == "0" ]] || { echo "expired unreferenced call record was not deleted" >&2; exit 1; }
[[ "$(postgres_value "SELECT count(*) FROM agent_messages WHERE call_id = '${saveliy_call}'")" == "0" ]] || { echo "expired Saveliy messages were not deleted" >&2; exit 1; }
[[ "$(postgres_value "SELECT count(*) FROM agent_messages WHERE call_id = '${konstantin_call}'")" == "1" ]] || { echo "Konstantin message history was not preserved" >&2; exit 1; }
[[ "$(postgres_value "SELECT count(*) FROM konstantin_messages WHERE call_id = '${konstantin_call}'")" == "1" ]] || { echo "Konstantin consultant history was not preserved" >&2; exit 1; }
[[ "$(postgres_value "SELECT count(*) FROM agent_context_summaries WHERE conversation_id = '${konstantin_conversation}'")" == "0" ]] || { echo "expired context summary was not cleaned independently" >&2; exit 1; }
[[ "$(postgres_value "SELECT count(*) FROM agent_calls WHERE call_id = '${konstantin_call}' AND request_json = '{}'::jsonb AND output IS NULL")" == "1" ]] || { echo "call tombstone did not preserve history references" >&2; exit 1; }
[[ "$(postgres_value "SELECT count(*) FROM agent_calls WHERE call_id = '${failed_news_call}' AND request_json->>'news_id' = '${unprocessed_news_id}' AND request_json->>'news_text' = 'unprocessed fixture text'")" == "1" ]] || { echo "terminal Serezha retry payload was removed before processing succeeded" >&2; exit 1; }
echo "ESB cleanup E2E passed (news, analytics, retries, chat history, call tombstones)."
