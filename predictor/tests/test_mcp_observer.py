#!/usr/bin/env python3
"""Unit tests for safe MCP tool-call observation and transparent forwarding."""

import importlib.util
import json
from pathlib import Path
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.request import Request, urlopen


OBSERVER_PATH = Path(__file__).with_name("mcp_observer.py")
SPEC = importlib.util.spec_from_file_location("mcp_observer", OBSERVER_PATH)
observer = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(observer)


class FakeESBHandler(BaseHTTPRequestHandler):
    def log_message(self, _format, *_args):
        return

    def do_POST(self):
        self.server.request_body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        self.server.authorization = self.headers.get("Authorization")
        payload = b'{"jsonrpc":"2.0","id":1,"result":{"content":[]}}'
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


class MCPObserverTest(unittest.TestCase):
    def test_fake_hermes_call_is_forwarded_and_only_safe_fields_are_recorded(self):
        fake_esb = ThreadingHTTPServer(("127.0.0.1", 0), FakeESBHandler)
        observer_server = ThreadingHTTPServer(("127.0.0.1", 0), observer.Handler)
        fake_esb.request_body = None
        fake_esb.authorization = None
        observer_server.upstream_url = f"http://127.0.0.1:{fake_esb.server_port}/mcp"
        observer.EVENTS.clear()
        threads = [
            threading.Thread(target=server.serve_forever, daemon=True)
            for server in (fake_esb, observer_server)
        ]
        for thread in threads:
            thread.start()
        try:
            request_body = json.dumps({
                "jsonrpc": "2.0",
                "id": 9,
                "method": "tools/call",
                "params": {"name": "get_company_profile", "arguments": {"company_id": "synthetic-id", "private": "must-not-be-stored"}},
            }).encode()
            request = Request(
                f"http://127.0.0.1:{observer_server.server_port}/mcp?agent=saveliy&correlation_id=test-correlation",
                data=request_body,
                headers={"Content-Type": "application/json", "Authorization": "Bearer test-secret"},
                method="POST",
            )
            with urlopen(request, timeout=3) as response:
                self.assertEqual(response.status, 200)
                self.assertEqual(response.read(), b'{"jsonrpc":"2.0","id":1,"result":{"content":[]}}')

            self.assertEqual(fake_esb.request_body, request_body)
            self.assertEqual(fake_esb.authorization, "Bearer test-secret")
            with urlopen(
                f"http://127.0.0.1:{observer_server.server_port}/events?correlation_id=test-correlation",
                timeout=3,
            ) as response:
                events = json.loads(response.read())["events"]
            self.assertEqual(events, [{
                "tool": "get_company_profile",
                "agent": "saveliy",
                "correlation_id": "test-correlation",
            }])
            self.assertEqual(set(events[0]), {"tool", "agent", "correlation_id"})
            self.assertNotIn("private", json.dumps(events))
            self.assertNotIn("test-secret", json.dumps(events))
        finally:
            for server in (observer_server, fake_esb):
                server.shutdown()
                server.server_close()
            for thread in threads:
                thread.join(timeout=3)


if __name__ == "__main__":
    unittest.main()
