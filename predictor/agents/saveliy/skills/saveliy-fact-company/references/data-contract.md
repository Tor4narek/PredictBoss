# Fact-to-company contract

- company_id: company scope for this chat and profile lookup.
- news_id: identifier used to fetch facts when they are not supplied in the call body.
- fact_id: existing ClickHouse fact identifier; never synthesize one.
- relevance: concise explanation of why the fact refers to the company.
- confidence: JSON number from 0 to 1 expressing confidence in the link. Do not quote it; use `0.85`, not `"0.85"`.

Fact-company links are stored separately from universal facts. The link does not change the underlying fact.
