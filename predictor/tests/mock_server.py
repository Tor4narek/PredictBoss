#!/usr/bin/env python3
"""Deterministic Hermes runtime mock for the ESB E2E stack."""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.request import Request, urlopen
from urllib.parse import urlparse

NEWS_SOURCE_ID = "22222222-2222-4222-8222-222222222222"
BANK_SOURCE_ID = "33333333-3333-4333-8333-333333333333"

RUNS = {}
IDEMPOTENCY = {}
REQUESTS = []
NEXT_OUTPUTS = {}
NEXT_STATUSES = {}
MCP_CALLS = []
STOP_REQUESTS = []
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

        if path == "/test/requests":
            with LOCK:
                requests = list(REQUESTS)
            self.send_json(200, {"requests": requests})
            return

        if path == "/test/tool-calls":
            with LOCK:
                calls = list(MCP_CALLS)
            self.send_json(200, {"calls": calls})
            return

        if path == "/test/stops":
            with LOCK:
                stops = list(STOP_REQUESTS)
            self.send_json(200, {"stops": stops})
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
        path = urlparse(self.path).path
        if path.startswith("/v1/runs/") and path.endswith("/stop"):
            if not self.authorized("test-hermes-api-key"):
                self.send_json(401, {"error": "unauthorized"})
                return
            run_id = path.removeprefix("/v1/runs/").removesuffix("/stop").rstrip("/")
            with LOCK:
                run = RUNS.get(run_id)
                if run is not None:
                    run["status"] = "cancelled"
                    STOP_REQUESTS.append({"run_id": run_id})
            if run is None:
                self.send_json(404, {"error": "run_not_found"})
            else:
                self.send_json(202, {"status": "stopping"})
            return
        if path == "/test/next-status":
            try:
                fixture = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
                company_id = fixture["company_id"]
                status = fixture["status"]
                request_kind = fixture.get("request_kind", "")
                if not isinstance(company_id, str) or status not in ("running", "completed"):
                    raise ValueError("invalid fixture")
                if request_kind not in ("", "analysis", "question"):
                    raise ValueError("invalid fixture")
            except (ValueError, TypeError, KeyError):
                self.send_json(400, {"error": "invalid_fixture"})
                return
            with LOCK:
                key = company_id
                if request_kind:
                    key = company_id + ":" + request_kind
                NEXT_STATUSES[key] = status
            self.send_json(201, {"status": "queued"})
            return
        if path == "/test/next-output":
            try:
                fixture = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
                company_id = fixture["company_id"]
                request_kind = fixture["request_kind"]
                output = fixture["output"]
                if not isinstance(company_id, str) or request_kind not in ("analysis", "question") or not isinstance(output, dict):
                    raise ValueError("invalid fixture")
            except (ValueError, TypeError, KeyError):
                self.send_json(400, {"error": "invalid_fixture"})
                return
            with LOCK:
                NEXT_OUTPUTS[(company_id, request_kind)] = json.dumps(output, separators=(",", ":"))
            self.send_json(201, {"status": "queued"})
            return
        if path == "/test/runs":
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
        if path != "/v1/runs":
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
            company_id = agent_input.get("company_id", "")
            request_kind = agent_input.get("request_kind", "")
            run_status = NEXT_STATUSES.pop(company_id + ":" + request_kind, None)
            if run_status is None:
                run_status = NEXT_STATUSES.pop(company_id, agent_input.get("mock_status", "completed"))
            if key:
                IDEMPOTENCY[key] = run_id

        kind = agent_input.get("request_kind")
        try:
            if "compact historical conversation context" in request.get("instructions", ""):
                output = "## Facts and identifiers\n- Preserve the conversation's known facts and identifiers.\n\n## Decisions\n- None\n\n## Open tasks\n- None\n\n## Constraints\n- Retain stated restrictions."
            elif kind in ("analysis", "question"):
                self.call_consultant_context(agent_input)
                with LOCK:
                    output = NEXT_OUTPUTS.pop((agent_input.get("company_id", ""), kind), None)
                if output is None:
                    if kind == "analysis":
                        text = "E2E analysis: market changes affect the company."
                    else:
                        text = "E2E answer: the source matters to the company's market."
                    output = json.dumps({"text": text}, separators=(",", ":"))
            elif os.environ.get("MOCK_SEREZHA_PERSIST") == "1" and "news_text" in agent_input and "source_id" in agent_input:
                self.save_serezha_news(agent_input)
                output = json.dumps({"status": "completed", "mock": True})
            else:
                output = json.dumps({"status": "completed", "mock": True})
        except Exception:
            with LOCK:
                if key and IDEMPOTENCY.get(key) == run_id:
                    del IDEMPOTENCY[key]
            self.send_json(502, {"error": "mock_runtime_failure"})
            return

        with LOCK:
            RUNS[run_id] = {
                "output": output,
                "status": run_status,
                "input": agent_input,
                "instructions": request.get("instructions", ""),
                "conversation_history": request.get("conversation_history", []),
            }
            REQUESTS.append({
                "instance_id": os.environ.get("MOCK_INSTANCE_ID", "mock"),
                "run_id": run_id,
                "idempotency_key": key,
            })
        self.send_json(202, {"run_id": run_id})

    def call_consultant_context(self, agent_input):
        call_context = agent_input.get("call_context", "")
        company_id = agent_input.get("company_id", "")
        if not call_context or not company_id:
            raise ValueError("mock consultant call lacks configured ESB context")
        context = self.call_mcp_tool("konstantin", "get_consultant_context", {
            "call_context": call_context, "company_id": company_id, "limit": 100,
        })
        with LOCK:
            MCP_CALLS.append({
                "request_id": agent_input.get("request_id", ""),
                "company_id": company_id,
                "tool": "get_consultant_context",
                "fact_count": len(context.get("facts", [])),
                "observation_count": len(context.get("observations", [])),
            })
        return context

    def save_serezha_news(self, agent_input):
        source_id = agent_input.get("source_id", "")
        news_id = agent_input.get("news_id", "")
        news_text = agent_input.get("news_text", "")
        if not source_id or not news_id or not news_text:
            raise ValueError("mock news call is incomplete")
        fact_text = "On " + str(agent_input.get("published_at", "")) + ": " + news_text
        self.call_mcp_tool("serezha", "save_news_and_facts", {
            "news_id": news_id,
            "source_id": source_id,
            "source_url": agent_input.get("source_url", ""),
            "company_id": agent_input.get("company_id", ""),
            "material_kind": agent_input.get("material_kind", "external_news"),
            "media_type": agent_input.get("media_type", "text/html"),
            "title": agent_input.get("title", ""),
            "published_at": agent_input.get("published_at"),
            "news_text": news_text,
            "facts": [{
                "fact_text": fact_text,
                "category": "news",
                "entities": [],
                "evidence": news_text,
                "occurred_at": agent_input.get("published_at"),
                "confidence": 0.99,
            }],
        })

    def call_mcp_tool(self, agent, tool_name, arguments):
        endpoint = os.environ.get("ESB_MCP_URL", "")
        token = os.environ.get("MCP_TOKEN_" + agent.upper(), "")
        if not endpoint or not token:
            raise ValueError("mock runtime lacks configured ESB MCP access")
        common_headers = {
            "Authorization": "Bearer " + token,
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "MCP-Protocol-Version": "2025-06-18",
        }

        def post(message, session_id=None):
            headers = dict(common_headers)
            if session_id:
                headers["Mcp-Session-Id"] = session_id
            request = Request(endpoint, data=json.dumps(message).encode(), headers=headers, method="POST")
            with urlopen(request, timeout=10) as response:
                body = response.read()
                return response, json.loads(body) if body else {}

        session_response, initialized = post({
            "jsonrpc": "2.0", "id": 1, "method": "initialize",
            "params": {
                "protocolVersion": "2025-06-18", "capabilities": {},
                "clientInfo": {"name": "predictor-e2e-mock-hermes", "version": "1.0"},
            },
        })
        session_id = session_response.headers.get("Mcp-Session-Id", "")
        protocol_version = initialized.get("result", {}).get("protocolVersion", "2025-06-18")
        common_headers["MCP-Protocol-Version"] = protocol_version
        if not session_id:
            raise ValueError("ESB MCP did not create a session")
        post({"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}, session_id)
        _, response = post({
            "jsonrpc": "2.0", "id": 3, "method": "tools/call",
            "params": {"name": tool_name, "arguments": arguments},
        }, session_id)
        result = response.get("result", {})
        if result.get("isError"):
            raise ValueError("ESB MCP tool call failed")
        structured = result.get("structuredContent")
        if isinstance(structured, dict):
            context = structured
        else:
            content = result.get("content", [])
            if content and isinstance(content[0].get("text"), str):
                decoded = json.loads(content[0]["text"])
                if isinstance(decoded, dict):
                    context = decoded
                else:
                    raise ValueError("ESB consultant context MCP response was not structured JSON")
            else:
                raise ValueError("ESB MCP response was not structured JSON")
        return context


def main():
    port = int(os.environ.get("MOCK_PORT", "8090"))
    server = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
