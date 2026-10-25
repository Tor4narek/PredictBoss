# Metric analysis contract

PostgreSQL is the source of truth for company metric settings, including type, characteristic, unit, and optional parameters. An observation links company_id and metric_id to a news_id in ClickHouse. It may contain a numeric value or null, a probability from 0 to 1, the inclusive period_from and period_to dates in YYYY-MM-DD form, and source_fact_ids.

Metric totals are calculated for a company and date range as the sum of value multiplied by probability. Observations with a null value contribute zero and are not included in observation_count. Totals are stored by company_id, metric_id, period_from, and period_to.

ClickHouse stores observations, calculated totals, and company analysis; it does not store metric definitions or characteristics. Company analysis stores probability, the metric IDs considered, the period, and readable analysis text. Probabilities represent uncertainty and are not guarantees. Agents access both stores only through ESB MCP tools.

Values and probabilities in MCP calls are JSON numbers, not quoted strings. Use JSON `null` when an observation value is unknown.
