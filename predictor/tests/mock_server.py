#!/usr/bin/env python3
"""Deterministic Hermes runtime mock for the ESB E2E stack."""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse

NEWS_SOURCE_ID = "22222222-2222-4222-8222-222222222222"
BANK_SOURCE_ID = "33333333-3333-4333-8333-333333333333"

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

        if path == "/test/runs":
            with LOCK:
                runs = [dict(run_id=run_id, **run) for run_id, run in RUNS.items()]
            self.send_json(200, {"runs": runs})
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
                self.send_json(200, {
                    "status": run.get("status", "completed"),
                    "output": run["output"],
                    "error": run.get("error", ""),
                })
            return

        self.send_json(404, {"error": "not_found"})

    def do_POST(self):
        global RUN_SEQUENCE
        if urlparse(self.path).path == "/test/runs":
            try:
                fixture = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
                run_id = fixture["run_id"]
                status = fixture["status"]
            except (ValueError, TypeError, KeyError):
                self.send_json(400, {"error": "invalid_fixture"})
                return
            with LOCK:
                RUNS[run_id] = {
                    "output": fixture.get("output", "{}"),
                    "status": status,
                    "error": fixture.get("error", ""),
                    "input": {},
                    "instructions": "test fixture",
                    "conversation_history": [],
                }
            self.send_json(201, {"run_id": run_id})
            return
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
            if "compact historical conversation context" in request.get("instructions", ""):
                output = "## Facts and identifiers\n- Preserve the conversation's known facts and identifiers.\n\n## Decisions\n- None\n\n## Open tasks\n- None\n\n## Constraints\n- Retain stated restrictions."
            elif kind in ("analysis", "question"):
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
            RUNS[run_id] = {
                "output": output,
                "input": agent_input,
                "instructions": request.get("instructions", ""),
                "conversation_history": request.get("conversation_history", []),
            }
            if key:
                IDEMPOTENCY[key] = run_id
        self.send_json(202, {"run_id": run_id})


def main():
    port = int(os.environ.get("MOCK_PORT", "8090"))
    server = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
