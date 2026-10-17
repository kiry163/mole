---
name: mole-agent
description: "Configure, extend, debug, or contribute to Mole itself. Use when the task is about the Mole CLI, config.json, the embedded web console, gateway, channels, models, skills, MCP servers, routing, sessions, built-in slash commands, or repository internals. Use Mole-native workflows, terminology, paths, and configuration."
metadata: {"nanobot":{"emoji":"🦞"}}
---

# Mole Agent

Mole is a lightweight personal AI assistant and agent framework shipped as a **single binary**. One process serves the embedded web console, the chat channels, the health endpoints and the MCP runtime on **one HTTP port** (default `localhost:18790`); there is no second process, no second binary and no second port. Alongside the gateway it offers a native CLI, installable skills, session routing and channel messaging.

`mole` with no arguments prints help; `mole serve` starts the gateway and mounts the console on the same port. The frontend assets are embedded into the binary, so starting Mole needs no Node build at runtime.

Use this skill when the job is about **Mole itself**: onboarding, configuration, debugging, adding features, extending the CLI, changing routing/session behavior, working on skills, channels or MCP support, or contributing to this repository.

Mole can run as a foreground process (`mole serve`) or as a user-level OS service (`mole service ...`). Both run the same executable, entry point and gateway; the service manager only owns process lifecycle.

## Operating Stance

When this skill is active, stay fully subordinate to Mole's real architecture:

- Prefer Mole commands, config keys, and workspace layout to ad-hoc workarounds.
- Follow Mole source for behavior, naming, and workflows.
- Treat repository code as the source of truth; this fork has no checked-in documentation tree.

## Quick Start

```bash
# Start the gateway and the embedded web console on one port
mole serve          # foreground process; prints its URL (default http://127.0.0.1:18790), opens no browser
mole serve -d       # same, with debug logging

# Or install the same process as a user-level service
mole service install --now   # write/enable the unit, start, and wait until ready
mole service status          # installed/running, PID, URL, health; never prints the token
mole service logs -f         # follow the platform service log

# Version information
mole version
```

**The console is the configuration UI.** A fresh home has no `config.json`; Mole starts anyway and
logs that it is in limited mode:

```text
⚠ Warning: no default model configured; gateway started in limited mode
```

Open the console, set the console access token on `/setup`, and add a model (endpoint, API key, enable
it, mark it default) from the console's model view. There is no first-run CLI step: every command that
used to write configuration (`mole onboard`, `mole model`, `mole config`, `mole auth`, `mole skills`,
`mole mcp`, `mole cron`, `mole status`, `mole agent`, `mole gateway`) is gone.

The equivalent hand-edit path is `~/.mole/config.json` plus `~/.mole/.env`:

- put the API key in the model entry's `api_keys` as a `$VAR` reference and define that variable in
  `~/.mole/.env` (the console does both automatically);
- set `"enabled": true` on the entry;
- point `agents.defaults.model_name` at that entry's `model_name`.

## Command Surface

```bash
mole                       # print help; does not start the gateway
mole serve [-d]            # run the gateway + console in the foreground; -d enables debug logging
mole service install [--now]
mole service reinstall [--now]
mole service uninstall
mole service start
mole service stop
mole service restart
mole service status [--json]
mole service logs [-f] [--lines 200]
mole service env refresh [--dry-run] [--restart]
mole version               # build/version information
```

`help` is a Cobra built-in (`mole help serve`); `completion` is disabled. Bare `mole service` prints
the service help. Anything else is not a command in this fork — configuration lives in the console
and in `config.json`, not in CLI verbs:

| Need | Where it lives now |
| --- | --- |
| Console login | `/setup` (first run) and `/login`; the token is `dashboard.token` (`"$DASHBOARD_TOKEN"` in `~/.mole/.env`) |
| Models | Console model view, or `model_list` + `agents.defaults.model_name` in `config.json` |
| Channels | Console channel view, or `channel_list` in `config.json` (secrets are `$VAR` references into `~/.mole/.env`) |
| Skills | Console skills view / the `install_skill` tool; roots are the workspace and global dirs |
| MCP servers | `tools.mcp.servers` in `config.json`, managed through the console config pages |
| Hooks | `hooks` in `config.json` (builtin hooks and external stdio processes), editable from the console's config page |
| Tool approval | `hooks.builtins.webui_approval.enabled` (on by default) switches the gate; `tools.approval` holds the policy; the queue lives at `/agent/approvals` |
| Runtime log | `~/.mole/logs/gateway.log`, also readable in the console |

### Weixin authentication

The Wechat (weixin) channel authenticates by scanning a QR code with the account that should act as
the bot. `pkg/channels/weixin/api.go` implements the QR exchange, but the CLI command that used to
drive it (`mole auth weixin`) was removed with the rest of the CLI: today there is **no** supported
entry point for the QR flow, so a weixin instance has to be provisioned out of band. Do not promise
`mole auth weixin` to a user of this fork.

## Service Lifecycle

`mole service` registers the running executable with the OS service manager for the **current
user**. It is not a separate daemon or a second configuration path: the unit launches `mole serve`
with `MOLE_HOME` (plus `MOLE_CONFIG` when set), and `pkg/service`'s program wrapper loads `.env`,
starts the gateway and waits for readiness exactly as the foreground command does. The service
definition carries no secrets and no `PATH`.

```text
install [--now]     write the unit, enable login start; default does not start; --now starts + waits /ready
reinstall [--now]   stop if running, remove, reinstall; restores the previous run state (or starts with --now)
uninstall           stop if running, remove the unit; MOLE_HOME data is kept
start               error if not installed; if running, print the URL and exit 0; else start + wait /ready
stop                stop the service and wait up to 45s for its PID file; never kills a manual serve
restart             restart the installed service and wait /ready
status [--json]     unit path, installed/running/manual, PID, URL, health, env freshness, config error
logs [-f]           tail the platform service log; --lines N (default 200)
env refresh         import selected shell variables into .env; --dry-run, --restart
```

`install` records the **absolute path** of the executable it is invoked from (a `go run`/`go-build`
temporary binary is rejected). Replace the binary at the same path and `service restart`; after
moving it, run `service reinstall`. When `.env` does not exist, `install` imports the same allowlisted
shell settings as `service env refresh` (`PATH`, `TZ`, `LANG`, `LC_ALL`, `HTTP_PROXY`,
`HTTPS_PROXY`, `NO_PROXY`, `ALL_PROXY`, `SSL_CERT_FILE`, `SSL_CERT_DIR`) and never overwrites an
existing file. It does **not** import secrets or arbitrary variables: model API keys, channel
tokens and `DASHBOARD_TOKEN` must be added to `.env` by hand or written by the console.

Status behavior worth quoting to users:

- `URL` is the console address from `gateway.host`/`gateway.port`; wildcard hosts are shown as
  loopback. `/setup` is appended while no console token is configured.
- The token is **never printed**; rotating or reading `dashboard.token` is a console/`.env` task.
- `Running: yes (manual)` plus `Manual: yes` means the PID belongs to `mole serve`, not to the
  service manager. `start`/`reinstall` refuse in that state; `stop` leaves it running and prints the
  PID.
- `Env changed: yes` means `.env` is newer than the running PID file: the process still uses the old
  values. Apply with `mole service restart` (or `mole service env refresh --restart`).
- `Config error:` is shown while the service state is still reported; fix `config.json` or `.env`
  and restart.

Platform backends and where their artifacts live:

| OS | Manager | Unit | Name (default / custom home) | Service log source |
| --- | --- | --- | --- | --- |
| macOS | launchd, `UserService` | `~/Library/LaunchAgents/<name>.plist` | `io.mole.agent` / `io.mole.agent.<8hex>` | `$MOLE_HOME/logs/<name>.out.log` and `.err.log`, then `gateway.log` |
| Linux | `systemd --user` | `~/.config/systemd/user/<name>.service` | `mole` / `mole-<8hex>` | `journalctl --user -u <name>`, then `gateway.log` |
| Windows | current-user Windows Service | service database (no file) | `Mole-<8hex>` (always hashed) | `$MOLE_HOME/logs/gateway.log` after start; if startup fails before the log exists, Windows Logs → System filtered to Service Control Manager (7000-series) |

macOS runs the LaunchAgent at load with keep-alive and 45s exit timeout; Linux uses
`WantedBy=default.target` with `Restart=on-failure` and `TimeoutStopSec=45`; Windows installs with
automatic start and restart-on-failure, prompts for the account password on the terminal (never
argv), and needs an elevated terminal to install.

### Environment refresh

`service env refresh` imports only `PATH`, `TZ`, `LANG`, `LC_ALL`, `HTTP_PROXY`, `HTTPS_PROXY`,
`NO_PROXY`, `ALL_PROXY`, `SSL_CERT_FILE` and `SSL_CERT_DIR` from the current shell into the instance
`.env`, preserving comments, secrets and unrelated lines. `--dry-run` prints the values without
writing; `--restart` performs the write, restarts the service and waits for `/ready`. There is no
hot reload: `.env` is read once in the program's `Start` before the gateway starts, so any manual
edit needs `mole service restart`.

### Multiple instances

Instance identity is the **absolute `MOLE_HOME`**. Each home gets its own service name, unit file,
PID file, `.env`, logs and `config.json`; several services can run side by side. Ports are **not**
allocated automatically: each instance binds `gateway.host`/`gateway.port` from its own config, so
set a distinct port per home before running them together. On macOS and Linux the default `~/.mole`
uses the unhashed name and any other absolute path appends the first 8 hex characters of a SHA-256
of the cleaned path; on Windows **every** home, including the default, is `Mole-<8hex>`. Setting a
different `MOLE_HOME` selects the instance for every command:

```bash
MOLE_HOME=/srv/mole-work mole service status
MOLE_HOME=/srv/mole-work mole service restart
```

`MOLE_HOME` and `MOLE_CONFIG` must be absolute for service commands (the manager starts the process
from an unrelated working directory). A manually started `mole serve` and an installed service for
the same home are mutually exclusive; the PID file is how the commands tell them apart. Different
homes are independent, including their workspaces and logs.

## Single Port, Web Console, and Channels

One binary, one process, one HTTP port. `mole serve` starts the gateway with the
embedded web console mounted on the same listener; there is no console-less mode any more.
The listener is `gateway.host`:`gateway.port` from `config.json`, defaulting to `localhost:18790`
(loopback). There are no command-line overrides: edit those two fields to bind elsewhere, then
restart `mole serve`.

Everything below shares that one port:

| Surface | Path |
| --- | --- |
| Health / readiness | `/health`, `/ready`, `/reload` (POST) |
| Console pages | `/login`, `/setup` (unauthenticated pages redirect to `/login`) |
| Webchat channel | `/webchat/` webhook, `/webchat/ws` websocket (`config.ChannelWebChat`) |

**Only `webchat` receives traffic on this port.** It is the console's channel: the browser talks to
the agent through `/webchat/`. The other four built-in channels connect outbound and open no
listener:

- `telegram` — long-polling bot API
- `qq` — bot gateway websocket
- `feishu` — websocket mode
- `weixin` — Tencent iLink REST API

### Channel Configuration

Channels live under `channel_list` in `config.json`, keyed by an instance name, each entry carrying
an explicit `"type"`. The built-in types are the five constants in `pkg/config/config_channel.go`:

```text
webchat   telegram   feishu   weixin   qq
```

A hand-written `config.json` needs an explicit entry (with `"type"`) for each channel you want; the console enables webchat automatically on first start. Secrets such as
`channel_list.telegram.settings.token` or `channel_list.webchat.settings.token` are written to
`~/.mole/.env` and referenced from `config.json` as `$VAR` (for example `"token": "$TELEGRAM_TOKEN"`).
Entries are only served when `"enabled": true`; with none enabled
the gateway still starts and logs `No channels enabled`.

The console itself is the browser UI for the running process: start it with `mole serve` and open
the URL that command prints in a browser (Mole does not open one itself). It exposes the chat, the model list,
the channel list, the agent views (hub, skills, tools, approvals), the configuration and the runtime
log. The console is gated by a console token stored as `dashboard.token` (set it on `/setup`,
enter it on `/login`).
Until a token exists, anyone who can reach the port can claim the setup page, which is why the
default bind is loopback and why a host that publishes the port must set the token immediately. The
token also keys the session cookie: a restart logs every browser out, and rotating the token does
not — sessions already signed in stay valid until the gateway restarts (the new value is accepted at
the next login). `SaveConfig` promotes it to `DASHBOARD_TOKEN` in `~/.mole/.env` (mode 0600), and
`config.json` keeps only the `$DASHBOARD_TOKEN` reference; there is no environment-variable override
and no `enc://`. The token never appears in console API responses, and the console's config view can
set it (writing `.env`) but cannot clear it.

### Tool-Call Approval

Mole can stop a tool call and wait for a human decision. This is the one hook that ships as a
built-in: it is mounted from `hooks.builtins.webui_approval.enabled` (`true` is already the
default in `DefaultConfig()`, so an existing config picks it up on the next start). It only applies
to the in-process tool path — the hook is mounted once per turn at hook initialisation, not per tool
call.

| Piece | Where |
| --- | --- |
| Switch | `hooks.builtins.webui_approval.enabled` (bool). `hooks.defaults.approval_timeout_ms` (default `60000`) is the deadline the hook manager applies. |
| Policy | `tools.approval`: `rules[]` (`{tool}` or `{category}` → `"allow"` / `"require"` / `"deny"`, first match wins) and `always_allow[]`. |
| Queue | The console page **Approvals** (`/agent/approvals`), fed by `GET /api/approvals` (1 s polling); the sidebar entry carries a badge with the pending count. |
| Decision | `POST /api/approvals/{id}` — the page's buttons are **Allow**, **Always allow** (only when there is a value to remember) and **Deny**. |

The default policy allows four categories — reads, searches, messages and sub-agent tools — and
requires approval for everything else, which includes `exec`, the write tools (`write_file` /
`edit_file` / `append_file`) and every `mcp_*` tool. A request that is not
answered inside `approval_timeout_ms` is denied (fail-closed) and disappears from the queue marked
as timed out; the turn continues with the denial rather than hanging.

**Always allow pins the argument.** The remember buttons are not equivalent to "trust this tool":
an `always_allow` entry is `{"tool": …, "arg": …, "value": …}`, where `exec` pins `command` and the
write tools pin `path`; everything else is remembered as a whole tool. So "always
allow `echo approved-tool-ran`" makes that exact command silent and still asks for a *different*
`exec` command. A call with nothing to remember (for example an `exec` call that carries no
`command`) offers only **Allow**/**Deny**, and asking again later is intended. The console writes
the entry through the config API's own validation (which rejects a non-string pinned value) and asks
for a reload, so a remembered entry takes effect on the running approver immediately, *unless a
reload is already in flight* — then the new reload request is dropped and the entry applies after
that reload finishes.
The pending queue survives any reload.

**Approval is interactive only.** There is no unattended mode: an approval request belongs to the
live turn that made the call and waits for a person to answer it in the console. If nobody answers
before `approval_timeout_ms`, the call is denied and the turn continues.

Two surfaces worth knowing when explaining what you see: a policy denial never enters the queue,
so it appears only in `/logs`, not in the page's "recent" list; and the
console chat renders tool *calls* but not tool *result* text, so a tool's output is found in the
session/log context rather than in the chat transcript.

## Providers

Mole talks to LLM providers through `model_list` entries; there is no separate provider daemon and
no CLI-provider subprocess path in this fork.

Credential patterns in Mole are:

- an API key on the `model_list` entry (`api_keys`), for every hosted API
- `api_base` plus an optional key for local or self-hosted endpoints

A `model_list` entry names the provider explicitly with `provider`, or relies on the default
protocol when `provider` is empty:

```json
{
  "model_name": "my-model",
  "provider": "deepseek",
  "model": "deepseek-chat",
  "api_keys": ["sk-..."],
  "enabled": true
}
```

- With `provider` set, Mole sends `model` to that provider unchanged.
- Without `provider`, a `provider/model` string is split only when the prefix is a known provider
  id; otherwise the whole string is treated as the model id and the `openai` protocol applies.
- `api_keys` is a list, so a model entry can fail over between several keys.

### Protocol Families

`pkg/providers/factory_provider.go` builds every **chat** provider from one of these protocol
families:

| Protocol family | Values accepted in `provider` |
| --- | --- |
| OpenAI-compatible HTTP | `openai`, `litellm`, `lmstudio`, `gpt4free`, `openrouter`, `groq`, `zhipu`, `nvidia`, `venice`, `nearai`, `ollama`, `moonshot`, `shengsuanyun`, `siliconflow`, `deepseek`, `cerebras`, `vivgrid`, `volcengine`, `vllm`, `qwen-portal`, `qwen-intl`, `qwen-us`, `mistral`, `avian`, `longcat`, `modelscope`, `novita`, `alibaba-coding`, `zai`, `mimo` |
| Anthropic HTTP | `anthropic` |
| Anthropic native Messages API | `anthropic-messages`, `alibaba-coding-anthropic` |

Anything else fails with `unknown protocol "<name>"`. `ollama`, `vllm`, `lmstudio` and `gpt4free`
are the protocols allowed to run without an API key.

### Provider Catalog

This is the full catalog the console offers (`pkg/providers/provider_metadata.go`); every entry is
configured the same way — `provider`, `model`, and an API key (in `api_keys` as a `$VAR` reference into `.env`):

| `provider` | Notes |
| --- | --- |
| `openai` | OpenAI API; default protocol when `provider` is omitted |
| `anthropic` | Anthropic over the OpenAI-compatible HTTP path |
| `anthropic-messages` | Anthropic's native Messages API |
| `deepseek` | DeepSeek |
| `openrouter` | OpenRouter |
| `qwen-portal` | Qwen / DashScope (China portal) |
| `qwen-intl` | Qwen international endpoint |
| `qwen-us` | Qwen US endpoint |
| `moonshot` | Moonshot / Kimi |
| `volcengine` | VolcEngine / Doubao |
| `zhipu` | Zhipu / GLM |
| `groq` | Groq |
| `mistral` | Mistral |
| `nvidia` | NVIDIA NIM |
| `cerebras` | Cerebras |
| `novita` | Novita AI |
| `shengsuanyun` | ShengsuanYun |
| `siliconflow` | SiliconFlow |
| `venice` | Venice AI |
| `nearai` | NEAR AI Cloud |
| `vivgrid` | Vivgrid |
| `longcat` | LongCat |
| `modelscope` | ModelScope |
| `avian` | Avian |
| `zai` | Z.ai |
| `alibaba-coding` | Alibaba Coding Plan |
| `alibaba-coding-anthropic` | Alibaba Coding Plan over the Anthropic protocol |
| `mimo` | Xiaomi MiMo |
| `gpt4free` | GPT4Free; no key required |
| `ollama` | Local Ollama, default `http://localhost:11434/v1`; no key required |
| `vllm` | Local vLLM, default `http://localhost:8000/v1`; no key required |
| `lmstudio` | Local LM Studio, default `http://localhost:1234/v1`; key optional |
| `litellm` | LiteLLM proxy; the proxy decides which credential model applies |

There is no `gemini`, `azure`, `bedrock`, `antigravity`, `github-copilot`, `claude-cli` or
`codex-cli` provider in this fork: Gemini and Azure-style endpoints are reached by pointing an
OpenAI-compatible entry at their `api_base`, and the local CLI-provider path has been removed.

## Built-in Tool Families

Mole tools are configured under `tools` in `config.json` and registered dynamically at runtime.

Important activation rules:

- most tools are enabled or disabled individually through `tools.*`
- agent-level allowlists can further restrict tool visibility
- `turn_profile.tools` can narrow tool exposure for a request or agent
- deferred MCP tools can stay hidden until unlocked by tool discovery

### Runtime Tool Families

| Family | Runtime tool names | What they provide |
| --- | --- | --- |
| Filesystem | `read_file`, `write_file`, `list_dir`, `edit_file`, `append_file` | Read, write, list, and patch workspace files |
| Web | `web_search`, `web_fetch` | Search the web and fetch readable page content |
| Command execution | `exec` | Shell command execution with deny-pattern guardrails |
| Skills registry | `find_skills`, `install_skill` | Search and install skills from configured registries |
| MCP | `mcp_<server>_<tool>` | Tools contributed by connected MCP servers |
| MCP discovery | `tool_search_tool_bm25`, `tool_search_tool_regex` | Discover deferred hidden MCP tools on demand |
| Messaging | `message`, `reaction` | Send outbound messages and reactions through channel integrations |
| Media | `send_file`, `load_image` | Send files and load local images into context |
| Subagents | `spawn`, `subagent`, `spawn_status`, `delegate` | Background tasks, synchronous sub-turns, task status, multi-agent delegation |

### Tool Registration Notes

- `spawn` and `spawn_status` require `subagent` support to be enabled.
- `delegate` is auto-registered only when more than one agent exists.
- MCP discovery tools are relevant only when deferred MCP discovery is enabled.
- `message` can be configured for outbound media as well as plain text.

Per-tool configuration lives under `tools` in `config.json`; the console's tools view renders the
same schema, and the per-tool struct tags in `pkg/config/config.go` are the authoritative field list.

## Specialized Subagents and Spawn

Mole has a first-class subagent model for long-running work, isolated subproblems, and multi-agent specialization.

The core idea is:

- use `spawn` for background work that should continue without blocking the current turn
- use `subagent` for an isolated synchronous sub-task when the parent needs the result now
- use `delegate` to hand a task to a specific peer agent with its own identity, model, workspace, and tools
- use `spawn_status` or `/subagents` to inspect what is currently running

### Choosing the Right Subagent Tool

| Tool | Execution style | Best use |
| --- | --- | --- |
| `spawn` | Async background task | Web research, API polling, long scans, work that can report back later |
| `subagent` | Sync isolated sub-turn | Focused analysis, transformation, or verification that must return before the parent continues |
| `delegate` | Sync handoff to named peer agent | Work that should run as a specialized agent rather than as a generic child turn |
| `spawn_status` | Inspection/status only | Check running, completed, failed, or canceled spawned tasks |

### Specialized Peer Agents

Specialized subagents are configured through the multi-agent system, not through ad hoc prompts alone.

The two important layers are:

- `config.json` defines which peer agents exist and which ones a given agent is allowed to spawn via `subagents.allow_agents`
- each agent's `AGENT.md` defines the identity that makes that peer worth spawning: `name`, `description`, tools, skills, MCP servers, and optional model overrides

Minimal shape:

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "default": true,
        "subagents": {
          "allow_agents": ["research"]
        }
      },
      {
        "id": "research"
      }
    ]
  }
}
```

Example `AGENT.md` for a specialist:

```md
---
name: Research Agent
description: Specialist for deep web research, evidence gathering, and synthesis.
tools: [web_search, web_fetch, message]
skills: [deep-research]
---
```

### Automatic Agent Discovery

When an agent has the `spawn` tool and at least one allowed peer, Mole injects a lightweight agent registry into the system prompt automatically.

That means:

- the model can see eligible peer agents without calling a separate `list_agents` tool
- only spawnable peers are shown
- the current agent is omitted
- discovery uses the peer agent's stable `id`, `name`, and `description`

In practice, this is what makes targeted `spawn(..., agent_id="research")` or `delegate(agent_id="research", ...)` reliable.

### Operational Behavior

Subagent execution semantics that matter:

- subagents run in isolated ephemeral session history, so their reasoning and intermediate steps do not pollute the parent conversation
- `spawn` returns immediately and launches background work in a goroutine
- `subagent` waits for completion and returns the result directly
- `delegate` is synchronous and runs as the target peer agent instead of a generic child task
- `spawn_status` is scoped to the current conversation when channel/chat context exists
- all subagents still share the same workspace security boundary; they do not bypass sandbox or path restrictions

Runtime limits and lifecycle rules:

- nested sub-turn depth is limited to 3
- concurrency is limited to 5 sub-turns per parent turn
- waiting for a concurrency slot times out after 30 seconds
- spawned background tasks are marked critical so they can survive graceful parent completion
- hard aborts still cascade to child and grandchild sub-turns

### Practical Patterns

Use `spawn` when:

- the task will take a while
- the result can arrive later
- the current turn should keep moving

Use `subagent` when:

- you need isolation from the parent context
- you want an independent attempt at a bounded subproblem
- the parent must wait for the answer before planning the next step

Use `delegate` when:

- a named peer agent is clearly better suited for the task
- that peer has a narrower tool/skill/model setup
- you want the task to run in the peer's own workspace/runtime identity

### Examples

Background specialist research:

```text
spawn(
  task="Search the web for the latest Mole MCP integration patterns and summarize them.",
  label="mcp-research",
  agent_id="research"
)
```

Synchronous isolated check:

```text
subagent(
  task="Review this config for risky tool exposure and return only the concrete findings."
)
```

Synchronous handoff to a named peer:

```text
delegate(
  agent_id="research",
  task="Collect three primary-source references for current provider authentication behavior."
)
```

### Observability

For live visibility:

- call `spawn_status` to inspect one task or list visible tasks in the current conversation
- use `/subagents` in chat channels to show the active subagent tree for the current session

## In-Session Slash Commands

Mole's shared slash command registry lives under `pkg/commands`.

Use these when helping users inside chat channels:

```text
/start
/help
/stop
/show model
/show channel
/show agents
/show mcp <server>
/list models
/list channels
/list agents
/list skills
/list mcp
/switch model to <name>
/check channel <name>
/clear
/context
/reload
/subagents
/use <skill> [message]
/use clear
/btw <question>
```

These are the thirteen built-in command groups in `pkg/commands/builtin.go`.

Semantics that matter:

- `/stop` cancels the task currently running in the session.
- `/switch model to <name>` changes the active model; `/switch channel` only points at `/check channel`.
- `/check channel <name>` verifies that a channel is available and enabled.
- `/clear` clears the chat history for the session.
- `/context` prints session message count and token usage (used, history, compress and summarize thresholds).
- `/reload` re-reads the configuration file.
- `/use <skill> <message>` forces one installed skill for a single request.
- `/use <skill>` arms that skill for the **next** message in the same chat session.
- `/use clear` (or `/use off`) cancels the pending skill override.
- `/btw <question>` asks an isolated side question without mutating the main session history.
- `/subagents` shows the currently active subagent tree for the session.
- Unknown slash commands pass through to normal LLM handling instead of hard-failing.

Telegram registers every built-in command group in its bot command menu (name plus description).

## Key Paths and Environment

### Important Files

```text
~/.mole/config.json            Main config
~/.mole/.env                   Instance secrets referenced as $VAR from config.json
~/.mole/logs/gateway.log       Gateway log written by `mole serve`
~/.mole/logs/gateway_panic.log Panic log
~/.mole/workspace/             Default workspace
~/.mole/workspace/skills/      Workspace skills (including the embedded skill set)
~/.mole/workspace/sessions/    Session history
~/.mole/workspace/state/       Runtime state
~/.mole/skills/                Global skills root
```

Service-managed installs add platform artifacts (on macOS/Linux, custom homes replace the unhashed
default with an 8-character hash of the absolute home; Windows always uses `Mole-<8hex>`):

```text
~/Library/LaunchAgents/io.mole.agent[.<8hex>].plist      macOS LaunchAgent unit
~/.config/systemd/user/mole[-<8hex>].service             Linux systemd --user unit
$MOLE_HOME/logs/io.mole.agent[.<8hex>].out.log           macOS service stdout
$MOLE_HOME/logs/io.mole.agent[.<8hex>].err.log           macOS service stderr
```

Windows keeps the definition in the service database (no unit file); post-start output is
`$MOLE_HOME/logs/gateway.log`. If the service fails before that file exists, SCM records the cause
in **Event Viewer → Windows Logs → System**, filtered to source **Service Control Manager**
(7000-series errors) and the `Mole-<8hex>` service; Mole writes no Application event-log records.
Use `mole service status` to print the exact `Unit:` path for the active home instead of guessing.

Default workspace layout:

```text
~/.mole/workspace/
├── AGENT.md            # agent definition prompt
├── SOUL.md             # persona, paired with AGENT.md
├── USER.md             # user profile
├── memory/MEMORY.md    # long-term memory
├── skills/             # skill roots (workspace skills win over the global root)
├── sessions/           # created at runtime
└── state/              # created at runtime
```

### Important Environment Variables

```bash
MOLE_CONFIG=/path/to/config.json
MOLE_HOME=/path/to/mole-home
MOLE_BUILTIN_SKILLS=/path/to/custom-builtin-skills
```

These three are bootstrap and path overrides; every other setting belongs to `config.json`, and
secrets are referenced from it as `$VAR` values defined in `~/.mole/.env`. Use `MOLE_CONFIG` when
the user reports "wrong config file" behavior. Use `MOLE_HOME` when the user wants a portable or
service-managed install. The gateway always logs to `~/.mole/logs/gateway.log`; set its verbosity
with `gateway.log_level` and the listen address with `gateway.host` / `gateway.port`.

## Mole-Native Concepts

### Model Configuration

Mole is model-centric. The key fields are:

- `agents.defaults.model_name`
- `model_list`
- optional `provider`
- runtime `model`

Important behavior:

- `agents.defaults.model_name` must match a `model_name` entry in `model_list`.
- If `provider` is set, Mole sends `model` to that provider unchanged.
- If `provider` is omitted, legacy `provider/model` parsing is still supported.

### Sessions and Routing

Session behavior is configured primarily through:

- `session.dimensions`
- `session.identity_links`
- `agents.dispatch.rules[*].session_dimensions`

Available dimensions:

- `space`
- `chat`
- `topic`
- `sender`

Baseline separation still includes:

- agent
- channel
- account

So even a tiny `session.dimensions` list does **not** create one giant global memory across every platform.

### Skills

Skills are plain directories with `SKILL.md`. The loader requires:

- a valid lowercase-or-hyphen skill name
- a non-empty description
- a `SKILL.md` file in the skill directory

Prefer this format:

```text
workspace/skills/<skill-name>/SKILL.md
```

Mole only relies on `name` and `description` frontmatter fields for loading and matching.

### MCP Discovery

Mole supports always-loaded and deferred MCP tools.

Use:

- `--deferred` when tools should stay hidden until explicitly discovered
- the console's tool view to inspect active tools
- `/list mcp` and `/show mcp <server>` from chat channels when debugging live agents

## Debugging Workflow

Start with the most Mole-native path:

1. Read `~/.mole/logs/gateway.log` (or the console's log view) first; for an installed service run
   `mole service logs --lines 200` or `mole service logs -f`.
2. Confirm which config file is active (`MOLE_CONFIG`, else `$MOLE_HOME/config.json`) and whether
   the process is the service or a manual `mole serve` (`mole service status`).
3. Inspect `agents.defaults.model_name` and `model_list`.
4. Restart with `mole serve -d` for runtime visibility (stop the installed service first if it owns
   the same home), or `mole service restart` for the managed process; the console's tool view shows
   live tools. The service has no debug flag, so use `gateway.log_level` or a foreground
   `mole serve -d` to reproduce.
5. For skill issues, inspect the skill directory and frontmatter.
6. For MCP issues, check `tools.mcp.servers` and the console's tool view.
7. For routing/session issues, inspect `session.dimensions` and `agents.dispatch.rules`.

Useful runtime facts:

- `mole serve -d` enables debug logging for the gateway and the console.
- gateway health endpoints expose `/health`, `/ready`, and `/reload`.
- `tool_feedback` can publish visible tool-execution notices directly into chats.

### Where to Find Logs

For gateway and runtime debugging, Mole writes logs under its home directory:

```text
~/.mole/logs/gateway.log
~/.mole/logs/gateway_panic.log
```

If `MOLE_HOME` is overridden, use:

```text
$MOLE_HOME/logs/gateway.log
$MOLE_HOME/logs/gateway_panic.log
```

In practice, check these places first:

- `~/.mole/logs/` for persisted gateway logs (the console's log view reads the same file)
- the terminal running `mole serve`
- for an installed service, `mole service logs` (launchd `*.out.log`/`*.err.log` on macOS,
  `journalctl --user -u <name>` on Linux, `gateway.log` on Windows); `-f` follows live output
- on Windows, when `gateway.log` does not exist because startup failed, **Event Viewer → Windows
  Logs → System**, filtered to source Service Control Manager (7000-series errors) and the
  `Mole-<8hex>` service name

Useful controls:

- `mole serve -d` for detailed runtime logs
- `mole service logs -f` for the managed process
- `gateway.log_level` to raise verbosity to `debug` or `info`

## Repository Map

When contributing code, these paths matter most:

- `cmd/mole/main.go` — root CLI wiring
- `cmd/mole/internal/serve/` — `mole serve`, which runs the gateway with the console mounted
- `cmd/mole/internal/service/` — `mole service` command tree: lifecycle verbs, status rendering, logs, env refresh
- `pkg/service/` — service spec/naming, kardianos/service config templates, program wrapper, status/URL/health, log tailing, `.env` bootstrap and refresh
- `cmd/mole/internal/cliui/`, `cmd/mole/internal/version/`, `cmd/mole/internal/helpers.go` — the rest of the CLI surface (colours, `mole version`, shared helpers)
- `pkg/dashboard/` — embedded console: HTTP server, REST API, SPA bundle, token login
- `pkg/gateway/` — process lifecycle, channel manager and log files
- `pkg/channels/` — channel implementations (`webchat`, `telegram`, `feishu`, `weixin`, `qq`)
- `pkg/providers/` — provider catalog, protocol factory, failover and rate limiting
- `pkg/tools/` — runtime tool implementations
- `pkg/commands/` — shared slash command registry
- `pkg/agent/` — prompt assembly, sessions, routing, tool execution, sub-turns, hooks
- `pkg/skills/` — skill loading, metadata, registry installs
- `pkg/mcp/` — MCP runtime integration
- `pkg/config/` — config schema, defaults, migration, persistence
- `web/frontend/` — sources for the SPA bundle embedded into `pkg/dashboard/dist`

## Contribution Rules

When changing Mole:

- Prefer extending existing CLI groups and shared registries instead of adding parallel one-off flows.
- Keep this skill and the root `README.md` aligned with code for CLI flags, slash commands, and config behavior — there is no other documentation tree in this fork.
- If you add or change a slash command, inspect `pkg/commands` and the chat-channel code that registers command availability.
- If you touch skills behavior, validate load order, naming rules, and frontmatter assumptions.
- If you touch routing or session logic, check `pkg/session/`, `pkg/routing/` and the session code in `pkg/agent/` together so behavior stays consistent.
- If you touch MCP, remember the CLI manages config while the runtime host manages execution.
- If you touch the channels or the console, remember the single-port rule: only `webchat` may claim a path on the shared mux.

## Common Troubleshooting

### "model ... not found in model_list"

Check that:

- `agents.defaults.model_name` matches a configured `model_name`
- the target `model_list` entry is enabled
- the `provider` and `model` fields use Mole's model-centric rules

### OpenRouter `free is not a valid model ID`

Prefer explicit provider config:

```json
{
  "provider": "openrouter",
  "model": "free"
}
```

Not:

```json
{
  "model": "free"
}
```

### Skill not appearing

Check:

- directory name is a valid skill name
- `SKILL.md` exists
- frontmatter `name` and `description` are present and sane
- the skill lives under workspace, global, or builtin roots

### MCP server exists but tools do not show up

Check:

- `tools.mcp.enabled` is true
- the server is enabled
- deferred discovery settings match expectations
- the server's tools appear in the console's tool view
- `/show mcp <server>` exposes tools from a chat channel

### Agent remembers too much or too little

Check:

- `session.dimensions`
- any per-rule `session_dimensions`
- whether the issue is really session isolation versus summarization

### Config edits do not seem to apply

Check:

- `MOLE_CONFIG`
- `MOLE_HOME`
- whether the user edited `config.json` or the matching `~/.mole/.env`
- whether they restarted the process after the edit (config and `.env` are read at startup):
  `mole service restart` for an installed service, or kill and relaunch `mole serve`
- `mole service status` — `Env changed: yes` means `.env` is newer than the running process

### Service commands fail or the service will not start

Check, in order:

- `mole service status` first: it reports `Installed`, `Running`, `Manual`, `PID`, `URL`, `Health`
  and `Config error` without loading the gateway, and never prints the token.
- `start`/`reinstall` saying a manually started instance is running means a `mole serve` PID file owns
  that home; stop it (`kill <PID>`) before starting the service.
- `service is not installed` is literal for `start`/`restart`; run `service install` first. After a
  binary moved, `service install` reports "already installed" — use `service reinstall`.
- Startup failures print the last service log lines and a hint about `gateway.host`/`gateway.port`;
  follow with `mole service logs -f`. A port conflict usually means another instance or a manual
  serve is bound to the same port under a different home.
- macOS `launchctl print gui/$(id -u)/<name>` and Linux `systemctl --user status <name>` show the
  manager's own view; `mole service logs` already selects the right source.
- On Windows, a failure before the gateway opens its log is recorded by the SCM in **Event Viewer
  → Windows Logs → System**, source **Service Control Manager** (7000-series), and names the
  `Mole-<8hex>` service; it does not appear in `gateway.log` or the Application log.
- `MOLE_HOME`/`MOLE_CONFIG` must be absolute for service installs; relative values are rejected
  because the manager starts the process outside the shell's working directory.
- Uninstalling keeps all data. Reinstalling recreates the unit from the current executable path and
  home.

## Where to Read Next

This fork ships no documentation tree besides this manual and `README.md` — the sources are the
record. Read these only when the task needs them:

- `pkg/config/config.go` + `pkg/config/defaults.go` for config keys, defaults and the example blocks
- `config/config.example.json` for a full annotated config
- `pkg/service/` for service naming, kardianos config templates, lifecycle, status, logs and env refresh
- `pkg/commands/*.go` for slash command names, usage strings and semantics
- `pkg/providers/provider_metadata.go` + `pkg/providers/factory_provider.go` for providers and protocols
- `pkg/channels/README.md` for the channel layer
- `pkg/dashboard/` for the console server, routes and token login
- `AGENT.md`, `SOUL.md`, `USER.md` in this workspace for what the agent itself is told

If the task is code-level rather than user-facing, read the matching package under
`pkg/commands/`, `pkg/agent/`, `pkg/skills/`, `pkg/channels/`, `pkg/dashboard/`
or `pkg/mcp/` before proposing behavior changes.
