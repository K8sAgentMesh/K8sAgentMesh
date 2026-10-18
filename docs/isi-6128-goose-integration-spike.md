# Goose integration spike — decision record (ISI-6128, AgentMesh P4.2 Phase A)

- **Date:** 2026-10-18
- **Status:** DECIDED — integration surface locked; adapter work unblocked (Phase B)
- **Scope:** lock the integration surface for the goose runtime before any adapter
  code is committed (issue mandate: *"do not commit the adapter before this decision"*).
- **Evidence:** all captures below are live probes of the **pinned binary v1.55.0**
  (`goose-x86_64-unknown-linux-musl.tar.gz`, static musl build, 43 MB compressed),
  run on claude-dev (10.0.0.189) against Mac Studio Ollama (10.0.0.185:11434,
  `qwen3.5:35b`). Raw artifacts: `~/.paperclip-spike/isi-6128/` on claude-dev
  (`run_stream_stdout.txt`, `run_json_stdout.txt`, `acp_probe.txt`, `serve_probe.txt`).

## Upstream ground truth (changed since plan rev 10)

1. **Repo moved:** `block/goose` → **`aaif-goose/goose`** (GitHub redirect confirmed
   2026-10-18). Latest release **v1.55.0** (2026-10-16). Pin this org+tag.
2. **`goose run` is first-class and machine-readable** — the "weaker guarantees than
   codex `--json`" concern from plan rev 10 is resolved: `--output-format <text|json|stream-json>`.
3. **ACP is now two first-class surfaces:** `goose acp` (ACP agent server on **stdio**)
   and `goose serve` (ACP over **HTTP/WebSocket**, default :3284, `GOOSE_SERVER__SECRET_KEY`
   auth, TLS, CORS origins). This supersedes the plan-rev-10 guess "`goose serve` speaks ACP on stdio".

## Live probe results (v1.55.0)

### 1. `goose run -i - --output-format stream-json` — the engine surface ✅

```
$ echo "Reply with exactly the single word: SPIKEOK" | \
  GOOSE_PROVIDER=ollama GOOSE_MODEL=qwen3.5:35b OLLAMA_HOST=http://10.0.0.185:11434 \
  goose run -i - --output-format stream-json --no-session --no-profile --max-turns 2
# exit 0, stderr empty; stdout =
    __( O)>  ● new session · ollama qwen3.5:35b      <- banner: first lines are NOT JSON
   \____)    20261018_1 · /home/hrexed/.paperclip-spike/isi-6128
     L L     goose is ready
{"type":"message","message":{"id":"chatcmpl-786","role":"assistant","created":...,
  "content":[{"type":"thinking","thinking":"The","signature":""}],
  "metadata":{"userVisible":true,"agentVisible":true,
  "inference":{"provider":"ollama","requestedModel":"qwen3.5:35b"},
  "operations":{"llm":{"advertised_tools":["load_skill"]}}}}}
{"type":"message",...,"content":[{"type":"text","text":"SPI"}]}           <- per-token deltas
{"type":"message",...,"content":[{"type":"text","text":"KE"}]}
{"type":"message",...,"content":[{"type":"text","text":"OK"}]}
{"type":"complete","total_tokens":10955,"input_tokens":10921,"output_tokens":34}
```

Properties that matter to the shim engine:

- **NDJSON event stream on stdout**; one JSON object per line; stderr stays clean.
- **Terminal event `{"type":"complete", ...}` with token usage** → natural
  `SettleLine`: settle when a line parses as JSON with `type == "complete"`.
  (Non-streaming `json` mode emits one doc with `metadata.status == "completed"`.)
- Message chunks are **OpenAI-chat-completion-shaped** with `content` blocks
  (`thinking` / `text` / tool blocks) — richer than codex: thinking is separable.
- **Banner preamble** (4 non-JSON lines) → parser must skip-until-first-JSON, not
  assume every line is JSON.
- **Prompt via stdin** (`-i -`) → prompt stays out of argv/process table (NFR-SEC1,
  same invariant as the codex wrapper and opencode `run`).
- **--no-session** avoids session persistence; sessions live in a sqlite
  `sessions.db` under the goose config dir — Phase B must isolate that dir per
  sandbox (see env contract).

### 2. `goose acp` (stdio) — proven, deferred ✅

```
$ printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,
  "clientCapabilities":{"fs":{"readTextFile":false,"writeTextFile":false}}}}' | goose acp
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,
  "agentCapabilities":{"loadSession":true,"promptCapabilities":{"image":true,"audio":false,
  "embeddedContext":true},"mcpCapabilities":{"http":true,"sse":false},
  "sessionCapabilities":{"list":{},"delete":{},"close":{}},"auth":{}, ...},
  "agentInfo":{"name":"goose","version":"1.55.0"}}}
```

Newline-delimited JSON-RPC 2.0 (no LSP framing). Works. Some sub-features are gated
at ACP protocol rev `2026-07-28` (e.g. `InputRequiredResult`) — the protocol is
actively evolving.

### 3. `goose serve` — ACP over HTTP/WebSocket

Up on :3284 (`GET /` → 404 with CORS headers; ACP rides dedicated routes/WS).
Auth via `GOOSE_SERVER__SECRET_KEY` (or `--dangerously-unauthenticated`). Recorded,
not adopted (see decision).

### 4. Env + config contract (confirmed from `--help` and binary strings)

| Var / flag | Value | Use |
|---|---|---|
| `GOOSE_PROVIDER` / `--provider` | openai, anthropic, **ollama**, databricks, gemini-cli, claude-code, … | credential + route mapping |
| `GOOSE_MODEL` / `--model` | any model id | model route |
| `GOOSE_MODE` | `auto` \| `approve` \| `smart_approve` \| `chat` | approval semantics; headless engine pins `auto` |
| `GOOSE_MAX_TURNS` / `--max-turns` | int | runaway bound (shim already enforces its own) |
| `GOOSE_TELEMETRY_OFF` | 1 | mandatory in sandbox |
| `GOOSE_DISABLE_KEYRING` | 1 | keyring-free sandboxes (secrets.yaml fallback) |
| `GOOSE_SYSTEM_PROMPT_FILE_PATH` | path | system-context delivery without argv |
| `--no-session`, `--no-profile`, `--quiet`, `--stats`, `--max-tool-repetitions` | flags | automation knobs |
| `--with-extension '[name:]ENV=… cmd args'`, `--with-builtin`, config.yaml `extensions` | MCP | **MCP-native**: stdio + streamable_http extension types |

Providers include claude-code and gemini-cli → goose can ride existing BYO
subscription credentials in later stories. Static **musl** build = no runtime deps
in the Kata sandbox (the "Rust static binary is a sandbox plus" note, now concrete).

## Decision

**Phase B integrates `goose run` (one-shot per Run, `--output-format stream-json`),
NOT ACP.** Split exactly like codex:

1. **k8squad shim engine** (`pkg/shim/runtimes/goose.go` + tests): `Command()` builds
   `goose run -i - --output-format stream-json --no-session --no-profile` with env
   `GOOSE_PROVIDER`/`GOOSE_MODEL`/`GOOSE_MODE=auto`/`GOOSE_TELEMETRY_OFF`/
   `GOOSE_DISABLE_KEYRING` (+ isolated config dir), prompt on stdin, `Parse()` maps
   NDJSON lines → Progress events (skip banner; coalesce per-token deltas; surface
   thinking/tool blocks as typed progress), `SettleLine` fires on `type == "complete"`,
   MCP IR rendered to goose `config.yaml` extensions as WorkDirFiles. Credential
   shape `api-key` (openai-compatible BYO endpoint or ollama lane; `Warmup` applies
   to the shared-LAN Ollama like opencode/codex).
2. **mesh `NewGooseAdapter`** (`mesh/goose.go`): `RuntimeSpec` + card/launch builders
   over `ChildProcessAdapter` — spawns `shim supervisor` with the goose flavor env,
   zero control-plane change (the P2 property). CLIVersion pinned `1.55.0`.
3. **k8squad runtime registry entry** + `Register()`.

**ACP is deferred, deliberately.** In the mesh architecture the *shim supervisor is
the standing process* the gateway manages; the engine's job is Run-scoped one-shot
execution. Speaking ACP to `goose acp`/`goose serve` would add a long-lived agent
session, JSON-RPC plumbing, and mid-run `request_permission` flows that duplicate
what the shim already owns — churn risk with zero P4.2 benefit. Re-open ACP only if
(a) upstream deprecates headless `run` (watch the client/server+ACP migration), or
(b) a later story wants gateway-direct goose without the shim. The handshake is
proven above, so that fallback is real, not theoretical.

## Risks (updated from plan rev 10)

- **Upstream churn is real** (org move already happened; ACP sub-features gated at
  2026-07-28) → pin `v1.55.0` musl artifact + SHA; re-verify surface on bump.
- Banner preamble + per-token deltas are parser obligations (skip-until-JSON,
  coalescing) — covered by parse tests.
- Tool-call event vocabulary not yet enumerated (probe prompts used no tools);
  Phase B parse tests must force tool loops and enumerate `tool_use`/`tool_result`
  block shapes before freezing `Parse()`.
- Config/session dir isolation per sandbox (`sessions.db`, `config.yaml`,
  `secrets.yaml`) — set an isolated HOME/config dir in `Command()`.

## Effort re-estimate (Phase B)

Engine + parse/settle tests ~1.5–2.5 d, mesh adapter ~0.5 d, registry ~0.5 d
→ **2.5–3.5 dev-days**, low end of the 5–8 d envelope (the self-describing NDJSON
stream with a terminal event is cheaper than opencode's 611-line engine).

## Repro

```bash
# on claude-dev, ~/.paperclip-spike/isi-6128/goose = v1.55.0 x86_64 musl static
GOOSE_PROVIDER=ollama GOOSE_MODEL=qwen3.5:35b OLLAMA_HOST=http://10.0.0.185:11434 \
  ./goose run -t "hello" --output-format stream-json --no-session --no-profile
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,
"clientCapabilities":{"fs":{"readTextFile":false,"writeTextFile":false}}}}' | ./goose acp
```
