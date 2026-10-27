#!/usr/bin/env python3
"""Zero-credential OpenAI-compatible stand-in for the CrewAI live lane.

The lane's acceptance target is the mesh→crew wire, not a model; the pinned
crew's litellm lane (OPENAI_BASE_URL/OPENAI_API_KEY) points here so the crew
executes for real — real agents, real kickoff, real A2A frames — against a
deterministic model answer.

Contract honored (what litellm's openai/* provider actually calls):
  POST /v1/chat/completions   — stream and non-stream responses
  GET  /v1/models             — model list probe

Behavior knobs:
  HOLD marker  — when the prompt text contains the token HOLD, the response
                 sleeps HOLD_SECONDS (env, default 8) first: the C1 reattach
                 and tasks/cancel scenarios need a task that stays in flight.
  Answer shape — fixed phrase WIRE_PROBE_FINAL_ANSWER; when the prompt
                 contains "marker: <word>", the word is echoed back so the
                 lane can prove the crew output flowed through this server.
"""

from __future__ import annotations

import json
import os
import re
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

WIRE_PROBE_FINAL_ANSWER = "wire-probe-final-answer"
HOLD_MARKER = "HOLD"
MODEL_ID = "gpt-4o-mini-lane"


def hold_seconds() -> float:
    try:
        return float(os.environ.get("HOLD_SECONDS", "8"))
    except ValueError:
        return 8.0


def answer_for(prompt: str) -> str:
    marker = ""
    m = re.search(r"marker:\s*([A-Za-z0-9_-]+)", prompt)
    if m:
        marker = f" {m.group(1)}"
    return f"{WIRE_PROBE_FINAL_ANSWER}{marker}"


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt: str, *args: object) -> None:
        sys.stderr.write(f"[mock-openai] {self.address_string()} {fmt % args}\n")

    def _json(self, code: int, payload: dict) -> None:
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/v1/models":
            self._json(200, {"object": "list", "data": [{"id": MODEL_ID, "object": "model"}]})
            return
        if self.path == "/healthz":
            self._json(200, {"ok": True})
            return
        self._json(404, {"error": {"message": f"no route {self.path}"}})

    def do_POST(self) -> None:  # noqa: N802
        if self.path != "/v1/chat/completions":
            self._json(404, {"error": {"message": f"no route {self.path}"}})
            return
        length = int(self.headers.get("Content-Length", "0"))
        try:
            req = json.loads(self.rfile.read(length) or b"{}")
        except json.JSONDecodeError:
            self._json(400, {"error": {"message": "bad json"}})
            return
        prompt = " ".join(
            part.get("text", "") if isinstance(part, dict) else str(part)
            for msg in req.get("messages", [])
            for part in ([msg.get("content")] if isinstance(msg.get("content"), str) else msg.get("content") or [])
        )
        sys.stderr.write(f"[mock-openai] prompt[:240]={prompt[:240]!r}\n")
        if HOLD_MARKER in prompt:
            sys.stderr.write(f"[mock-openai] holding {hold_seconds()}s (HOLD prompt)\n")
            time.sleep(hold_seconds())
        answer = answer_for(prompt)
        created = int(time.time())
        if req.get("stream"):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            for token in (answer[:8], answer[8:16], answer[16:]):
                chunk = {
                    "id": "chatcmpl-lane",
                    "object": "chat.completion.chunk",
                    "created": created,
                    "model": req.get("model", MODEL_ID),
                    "choices": [{"index": 0, "delta": {"content": token}, "finish_reason": None}],
                }
                self.wfile.write(f"data: {json.dumps(chunk)}\n\n".encode())
                self.wfile.flush()
                time.sleep(0.05)
            done = {
                "id": "chatcmpl-lane",
                "object": "chat.completion.chunk",
                "created": created,
                "model": req.get("model", MODEL_ID),
                "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
            }
            self.wfile.write(f"data: {json.dumps(done)}\n\n".encode())
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
            return
        self._json(
            200,
            {
                "id": "chatcmpl-lane",
                "object": "chat.completion",
                "created": created,
                "model": req.get("model", MODEL_ID),
                "choices": [
                    {
                        "index": 0,
                        "message": {"role": "assistant", "content": answer},
                        "finish_reason": "stop",
                    }
                ],
                "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
            },
        )


def main() -> int:
    port = int(sys.argv[sys.argv.index("--port") + 1]) if "--port" in sys.argv else 0
    srv = ThreadingHTTPServer(("127.0.0.1", port), Handler)
    sys.stderr.write(f"LANE_MOCK_READY port={srv.server_address[1]}\n")
    sys.stderr.flush()
    srv.serve_forever()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
