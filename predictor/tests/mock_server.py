#!/usr/bin/env python3
"""Deterministic Domain API and Hermes runtime mock for the ESB E2E stack."""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import unquote, urlparse

COMPANY_ID = "11111111-1111-4111-8111-111111111111"
NEWS_SOURCE_ID = "22222222-2222-4222-8222-222222222222"
BANK_SOURCE_ID = "33333333-3333-4333-8333-333333333333"
NEW_SOURCE_ID = "44444444-4444-4444-8444-444444444444"

SOURCES = {
    NEWS_SOURCE_ID: {
        "sourceId": NEWS_SOURCE_ID,
        "companyId": COMPANY_ID,
        "canonicalUrl": "https://example.test/news/market-change",
        "materialKind": "external_news",
        "mediaType": "text/html",
        "title": "Market change",
        "publishedAt": "2026-10-25T09:00:00Z",
    },
    BANK_SOURCE_ID: {
        "sourceId": BANK_SOURCE_ID,
        "companyId": COMPANY_ID,
        "canonicalUrl": "https://example.test/alpha/market-analysis.pdf",
        "materialKind": "alpha_analysis",
        "mediaType": "application/pdf",
        "title": "Alpha market analysis",
        "publishedAt": "2026-10-25T10:00:00Z",
    },
    NEW_SOURCE_ID: {
        "sourceId": NEW_SOURCE_ID,
        "companyId": COMPANY_ID,
        "canonicalUrl": "https://example.test/news/new-source",
        "materialKind": "external_news",
        "mediaType": "text/html",
        "title": "New source",
        "publishedAt": "2026-10-25T11:00:00Z",
    },
}

RUNS = {}
IDEMPOTENCY = {}
LOCK = threading.Lock()
RUN_SEQUENCE = 0


class Handler(BaseHTTPRequestHandler):
    server_version = "PredictorE2EMock/1.0"

    def log_message(self, _format, *_args):
        return

    def send_json(self, status, value):
        encoded = json.dumps(value, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def authorized(self, token):
        return self.headers.get("Authorization") == "Bearer " + token

    def do_GET(self):
        path = urlparse(self.path).path
        if path == "/health":
            self.send_json(200, {"status": "ok"})
            return

        if path.startswith("/internal/news-links/"):
            if not self.authorized("test-domain-api-token"):
                self.send_json(401, {"error": "unauthorized"})
                return
            source_id = unquote(path.rsplit("/", 1)[-1])
            source = SOURCES.get(source_id)
            if source is None:
                self.send_json(404, {"error": "not_found"})
            else:
                self.send_json(200, source)
            return

        if path.startswith("/v1/runs/"):
            if not self.authorized("test-hermes-api-key"):
                self.send_json(401, {"error": "unauthorized"})
                return
            run_id = path.rsplit("/", 1)[-1]
            with LOCK:
                run = RUNS.get(run_id)
            if run is None:
                self.send_json(404, {"error": "run_not_found"})
            else:
                self.send_json(200, {"status": "completed", "output": run["output"]})
            return

        self.send_json(404, {"error": "not_found"})

    def do_POST(self):
        global RUN_SEQUENCE
        if urlparse(self.path).path != "/v1/runs":
            self.send_json(404, {"error": "not_found"})
            return
        if not self.authorized("test-hermes-api-key"):
            self.send_json(401, {"error": "unauthorized"})
            return
        try:
            request = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            agent_input = json.loads(request.get("input", "{}"))
        except (ValueError, TypeError):
            self.send_json(400, {"error": "invalid_request"})
            return

        key = self.headers.get("Idempotency-Key", "")
        with LOCK:
            existing = IDEMPOTENCY.get(key) if key else None
            if existing:
                self.send_json(200, {"run_id": existing})
                return
            RUN_SEQUENCE += 1
            run_id = "e2e-run-%d" % RUN_SEQUENCE
            kind = agent_input.get("request_kind")
            if kind in ("analysis", "question"):
                if kind == "analysis":
                    text = "E2E analysis: market changes affect the company."
                else:
                    text = "E2E answer: the source matters to the company's market."
                output = json.dumps({
                    "text": text,
                    "evidence": [
                        {"sourceId": NEWS_SOURCE_ID, "originKind": "fact"},
                        {"sourceId": BANK_SOURCE_ID, "originKind": "bank_view"},
                    ],
                })
            else:
                output = json.dumps({"status": "completed", "mock": True})
            RUNS[run_id] = {"output": output, "input": agent_input}
            if key:
                IDEMPOTENCY[key] = run_id
        self.send_json(202, {"run_id": run_id})


def main():
    port = int(os.environ.get("MOCK_PORT", "8090"))
    server = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
