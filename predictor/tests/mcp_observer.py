#!/usr/bin/env python3
"""Test-only transparent Streamable HTTP MCP observer for opt-in LLM tests."""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError, URLError
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit
from urllib.request import Request, urlopen

EVENTS = []
EVENTS_LOCK = threading.Lock()
MAX_EVENTS = 10_000
FORWARDED_HEADERS = {
    "accept",
    "authorization",
    "content-type",
    "last-event-id",
    "mcp-protocol-version",
    "mcp-session-id",
    "origin",
}
RESPONSE_HEADERS = {
    "cache-control",
    "content-type",
    "mcp-protocol-version",
    "mcp-session-id",
    "retry-after",
}


def record_tool_calls(body, agent, correlation_id):
    """Record only safe identifying fields from JSON-RPC tools/call messages."""
    messages = body if isinstance(body, list) else [body]
    events = []
    for message in messages:
        if not isinstance(message, dict) or message.get("method") != "tools/call":
            continue
        params = message.get("params")
        tool_name = params.get("name") if isinstance(params, dict) else None
        if isinstance(tool_name, str) and tool_name:
            events.append({"tool": tool_name, "agent": agent, "correlation_id": correlation_id})
    if events:
        with EVENTS_LOCK:
            EVENTS.extend(events)
            del EVENTS[:-MAX_EVENTS]
    return events


class Handler(BaseHTTPRequestHandler):
    server_version = "PredictorMCPTestObserver/1.0"

    def log_message(self, _format, *_args):
        return

    def do_GET(self):
        if urlsplit(self.path).path == "/health":
            self._json(200, {"status": "ok"})
            return
        if urlsplit(self.path).path == "/events":
            self._events()
            return
        self._proxy()

    def do_POST(self):
        self._proxy()

    def do_DELETE(self):
        self._proxy()

    def _events(self):
        query = dict(parse_qsl(urlsplit(self.path).query))
        correlation_id = query.get("correlation_id", "")
        with EVENTS_LOCK:
            events = [event.copy() for event in EVENTS if event["correlation_id"] == correlation_id]
        self._json(200, {"events": events})

    def _proxy(self):
        parsed = urlsplit(self.path)
        correlation = dict(parse_qsl(parsed.query))
        agent = correlation.pop("agent", "")
        correlation_id = correlation.pop("correlation_id", "")
        query = urlencode(correlation)
        target = urlsplit(self.server.upstream_url)
        upstream_url = urlunsplit((target.scheme, target.netloc, target.path, query, ""))
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length) if length else None
        if body:
            try:
                record_tool_calls(json.loads(body), agent, correlation_id)
            except (UnicodeDecodeError, json.JSONDecodeError):
                pass

        headers = {
            name: value
            for name, value in self.headers.items()
            if name.lower() in FORWARDED_HEADERS
        }
        request = Request(upstream_url, data=body, headers=headers, method=self.command)
        try:
            response = urlopen(request, timeout=300)
        except HTTPError as error:
            response = error
        except (OSError, URLError):
            self._json(502, {"error": "mcp_upstream_unavailable"})
            return
        with response:
            self.send_response(response.status)
            for name, value in response.headers.items():
                if name.lower() in RESPONSE_HEADERS:
                    self.send_header(name, value)
            content_length = response.headers.get("Content-Length")
            if content_length is not None:
                self.send_header("Content-Length", content_length)
            self.end_headers()
            while chunk := response.read(64 * 1024):
                self.wfile.write(chunk)
                self.wfile.flush()

    def _json(self, status, value):
        encoded = json.dumps(value, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)


def main():
    upstream = os.environ.get("MCP_OBSERVER_UPSTREAM", "http://esb:8081/mcp")
    port = int(os.environ.get("MCP_OBSERVER_PORT", "8095"))
    server = ThreadingHTTPServer(("0.0.0.0", port), Handler)
    server.upstream_url = upstream
    server.serve_forever()


if __name__ == "__main__":
    main()
