# Fact-to-company contract

- company_id: company scope for this chat and profile lookup.
- news_id: identifier used to fetch facts when they are not supplied in the call body.
- fact_id: existing ClickHouse fact identifier; never synthesize one.
- relevance: concise explanation of why the fact refers to the company.
- confidence: number from 0 to 1 expressing confidence in the link.

Fact-company links are stored separately from universal facts. The link does not change the underlying fact.
