#!/usr/bin/env python3
"""Live-lane CrewAI A2A server — the pinned stack for real.

Assembles the crew the way a crew project does on the pinned stack
(crewai[a2a]==1.15.28 → a2a-sdk~=0.3.10, Google A2A 0.3.0): a real Crew
(real Agent, real Task, real kickoff through the litellm lane) behind the
a2a-sdk JSON-RPC+SSE server on the default binding — POST /a2a with
message/stream SSE, tasks/get, tasks/cancel, and the agent card at
/.well-known/agent-card.json. Bearer auth on /a2a reads AUTH_TOKEN from the
process env (SimpleTokenAuth semantics, the contract mesh/crewai.go locks).

Lane instrumentation (fixture-side, mirroring the fake server's counters so
the Go lane can assert C1 "no double kickoff" and tasks/cancel against the
real server):
  GET /lane/stats — {"kickoffs": N, "cancels": N, "active": 0|1}
"""

from __future__ import annotations

import asyncio
import os
import sys
import threading
import uuid

import uvicorn
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Route

from a2a.server.agent_execution import AgentExecutor, RequestContext
from a2a.server.apps import A2AStarletteApplication
from a2a.server.events import EventQueue
from a2a.server.request_handlers.default_request_handler import (
    DefaultRequestHandler,
)
from a2a.server.tasks import InMemoryTaskStore
from a2a.types import (
    AgentCapabilities,
    AgentCard,
    Artifact,
    Part,
    TaskArtifactUpdateEvent,
    TaskState,
    TaskStatus,
    TaskStatusUpdateEvent,
    TextPart,
)

STATS = {"kickoffs": 0, "cancels": 0, "active": 0}
STATS_LOCK = threading.Lock()


def _bump(key: str, delta: int = 1) -> None:
    with STATS_LOCK:
        STATS[key] += delta


def _set(key: str, value: int) -> None:
    with STATS_LOCK:
        STATS[key] = value


class CrewBridgeExecutor(AgentExecutor):
    """Bridges the module a2a task onto a real crew kickoff.

    The Google A2A message text arrives exactly as mesh/crewaiwire.go composes
    it (system context + input); the crew runs for real and the kickoff output
    rides the artifact surface the router maps to EventArtifactRef.
    """

    def __init__(self, crew_factory) -> None:
        self._crew_factory = crew_factory

    def _ids(self, context: RequestContext) -> tuple[str, str]:
        return (
            context.task_id or str(uuid.uuid4()),
            context.context_id or str(uuid.uuid4()),
        )

    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        text = ""
        msg = context.message
        if msg is not None and getattr(msg, "parts", None):
            def _part_text(p):
                part = getattr(p, "root", p)
                return part.text if getattr(part, "kind", "") == "text" else ""
            text = "\n".join(filter(None, (_part_text(p) for p in msg.parts)))
        task_id, context_id = self._ids(context)
        _bump("kickoffs")
        _set("active", 1)
        await event_queue.enqueue_event(
            TaskStatusUpdateEvent(
                taskId=task_id,
                contextId=context_id,
                status=TaskStatus(state=TaskState.working),
                final=False,
            )
        )
        loop = asyncio.get_running_loop()
        try:
            crew = self._crew_factory()
            result = await asyncio.to_thread(crew.kickoff, inputs={"topic": text or "the wire lane"})
        finally:
            _set("active", 0)
        output = str(getattr(result, "raw", None) or result)
        await event_queue.enqueue_event(
            TaskArtifactUpdateEvent(
                taskId=task_id,
                contextId=context_id,
                artifact=Artifact(
                    artifactId=str(uuid.uuid4()),
                    name="crew-output",
                    parts=[Part(root=TextPart(text=output))],
                ),
                lastChunk=True,
                append=False,
            )
        )
        await event_queue.enqueue_event(
            TaskStatusUpdateEvent(
                taskId=task_id,
                contextId=context_id,
                status=TaskStatus(state=TaskState.completed),
                final=True,
            )
        )

    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        _bump("cancels")
        task_id, context_id = self._ids(context)
        await event_queue.enqueue_event(
            TaskStatusUpdateEvent(
                taskId=task_id,
                contextId=context_id,
                status=TaskStatus(state=TaskState.canceled),
                final=True,
            )
        )


def build_crew():
    from crewai import Agent, Crew, Task

    agent = Agent(
        role="Wire Lane Prober",
        goal="Answer every request with the exact probe phrase the model returns.",
        backstory="A deterministic fixture agent in the K8sAgentMesh live lane.",
        llm="openai/gpt-4o-mini",
        allow_delegation=False,
        verbose=False,
        max_iter=2,
    )
    task = Task(
        description="Answer the request about {topic}. Reply with the probe phrase the model gives you, nothing else.",
        expected_output="The probe phrase.",
        agent=agent,
    )
    return Crew(agents=[agent], tasks=[task], verbose=False, memory=False)


def lane_stats(_request):
    with STATS_LOCK:
        return JSONResponse(dict(STATS))


class BearerAuth:
    """Pure-ASGI bearer check on POST /a2a — SimpleTokenAuth semantics."""

    def __init__(self, app, token: str | None) -> None:
        self.app = app
        self.token = token

    async def __call__(self, scope, receive, send):
        if scope["type"] == "http" and scope["path"] == "/a2a" and scope["method"] == "POST":
            headers = {k.decode().lower(): v.decode() for k, v in scope.get("headers", [])}
            supplied = headers.get("authorization", "")
            if self.token and supplied != f"Bearer {self.token}":
                await JSONResponse({"error": "unauthorized"}, status_code=401)(scope, receive, send)
                return
        await self.app(scope, receive, send)


def main() -> int:
    port = int(sys.argv[sys.argv.index("--port") + 1]) if "--port" in sys.argv else 8090
    if "--llm-url" in sys.argv:
        os.environ["OPENAI_BASE_URL"] = sys.argv[sys.argv.index("--llm-url") + 1]
    os.environ.setdefault("OPENAI_API_KEY", "lane-dummy")
    os.environ.setdefault("CREWAI_TELEMETRY_OPT_OUT", "true")

    card = AgentCard(
        name="lane-crew",
        description="K8sAgentMesh live-lane CrewAI fixture crew",
        url=f"http://127.0.0.1:{port}/",
        version="1.0.0",
        capabilities=AgentCapabilities(streaming=True, pushNotifications=False),
        defaultInputModes=["text/plain"],
        defaultOutputModes=["text/plain"],
        skills=[],
        protocolVersion="0.3.0",
        preferredTransport="JSONRPC",
    )
    handler = DefaultRequestHandler(
        agent_executor=CrewBridgeExecutor(build_crew),
        task_store=InMemoryTaskStore(),
    )
    app = A2AStarletteApplication(agent_card=card, http_handler=handler).build(
        rpc_url="/a2a"  # crewai ServerTransportConfig.jsonrpc rpc_path default (the locked contract)
    )
    if isinstance(app, Starlette):
        app.routes.append(Route("/lane/stats", lane_stats))
    authed = BearerAuth(app, os.environ.get("AUTH_TOKEN"))

    config = uvicorn.Config(authed, host="127.0.0.1", port=port, log_level="warning")

    async def serve() -> None:
        sys.stderr.write(f"LANE_CREW_READY port={port} auth={'on' if os.environ.get('AUTH_TOKEN') else 'off'}\n")
        sys.stderr.flush()
        await uvicorn.Server(config).serve()

    asyncio.run(serve())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
