# Mole

**Mole** is a lightweight personal AI assistant written in Go that ships as a single binary: it serves the web console and the `webchat` channel from one process on one port.

---

## Build

Two build paths:

| Command | What it does |
|---------|--------------|
| `make build` | Builds the console frontend first (`web/frontend`, via `pnpm` — **Node.js and pnpm are required**), then the Go binary. Result: `./mole`. |
| `go build ./cmd/mole` | Go toolchain only, **no Node.js needed**. It embeds whatever is already in `pkg/dashboard/dist`; in a fresh clone that directory contains only the tracked `.gitkeep` placeholder, so the binary runs but the console has no UI until you build the frontend. |

## Quick start

```bash
mole serve   # starts the gateway + web console in the foreground; no browser opens
```

`mole` without arguments prints help. Open the console manually at the address `serve` prints
(or `http://127.0.0.1:18790` with the default `gateway.host`/`gateway.port`).

For an always-on install that starts at login, register the same `mole serve` process with the OS
service manager instead of keeping a terminal open — see [Run as a service](#run-as-a-service).

On a machine with no model configured yet the console starts in limited mode: it logs that no default model is set and serves its first-run setup page, where you set a console access token and add a model with its API key. The token is the config's `dashboard.token`: saving it writes `DASHBOARD_TOKEN` to `~/.mole/.env` (mode 0600) and leaves only the `$DASHBOARD_TOKEN` reference in `config.json`, and console API responses omit it entirely. Make it a long random string. The token also keys the session cookie: restarting the gateway signs every browser out, while rotating the token does not - sessions already signed in stay valid until you restart, although the new value is accepted at the next login. To preconfigure by hand instead, edit `~/.mole/config.json` (see `config/config.example.json`) and put the secrets it references into `~/.mole/.env` (see `config/env.example`): give one `model_list` entry an `api_keys` reference such as `$DEEPSEEK_API_KEY`, set `"enabled": true` on it and select it via `agents.defaults.model_name`.

## Run as a service

`mole service` installs the agent as a **user-level** service that starts at login and is restarted
if it exits. The service runs the same executable and the same `serve` entry point as the
foreground command, with `MOLE_HOME` (and `MOLE_CONFIG`, when set) baked into the service
definition. Secrets are not: they stay in the instance `.env`, which the program loads before the
gateway starts.

```bash
mole service install          # write and enable the service; does not start it
mole service install --now    # install, then start and wait until /ready
mole service status           # installed/running/PID/URL/health; never prints the token
mole service start            # start and wait until /ready
mole service restart          # restart and wait until /ready
mole service stop             # stop and wait for the PID file to disappear
mole service uninstall        # stop, remove the definition; MOLE_HOME data is kept
mole service logs [-f] [--lines 200]
mole service env refresh [--dry-run] [--restart]
mole service reinstall [--now]  # replace the definition, restoring the previous run state
```

`install` records the absolute path of the running executable, so build/install the binary at its
final location before installing; after replacing it at the same path, `service restart` picks up
the new build. `status --json` is available for scripts. `status` prints the console URL, using the
loopback address when `gateway.host` is a wildcard, and appends `/setup` while no console token is
configured. It never prints the token.

Platform backends:

- **macOS** — a per-user LaunchAgent in `~/Library/LaunchAgents/` (`io.mole.agent.plist`, or
  `io.mole.agent.<8hex>.plist` for a non-default home). It runs at load, is kept alive, and writes
  `*.out.log`/`*.err.log` into `$MOLE_HOME/logs/`.
- **Linux** — a `systemd --user` unit in `~/.config/systemd/user/` (`mole.service`, or
  `mole-<8hex>.service` for a non-default home), `WantedBy=default.target`, restarted on failure,
  logging to the user journal (`journalctl --user -u mole`).
- **Windows** — a service in the current user's account (installing requires an elevated terminal;
  the account password is prompted for and is never taken from the command line). It starts
  automatically and restarts on failure; post-start logs go to `$MOLE_HOME/logs/gateway.log`. If
  the service fails to start before the gateway can create that file (bad executable path, start
  type, or account permission), check **Event Viewer → Windows Logs → System**, filtered to source
  **Service Control Manager** (7000-series errors) and the `Mole-<8hex>` service name. Mole writes
  no Application event-log records.

**Environment.** The service manager launches `mole` with a minimal environment, so
`service install` imports the same allowlisted shell settings into the instance `.env` when the file
does not exist yet (an existing file is never overwritten), and `service env refresh` re-imports
them. The allowlist is `PATH`, `TZ`, `LANG`, `LC_ALL`, `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`,
`ALL_PROXY`, `SSL_CERT_FILE` and `SSL_CERT_DIR`; other lines are preserved. **Secrets — model API
keys, channel tokens, the console token — are never imported: add them to `.env` by hand**, or let
the console write them. `.env` is read at service start: edit it, then run
`mole service restart` (or `mole service env refresh --restart` to refresh and restart in one
step). `service status` reports `Env changed: yes` when `.env` is newer than the running process,
and `--dry-run` shows what refresh would write without changing the file. A `mole serve` process
started manually and an installed service cannot share the same home; the service commands detect
the manual PID file and refuse to start or stop it.

Different absolute `MOLE_HOME` directories derive different service names, so several agents can be
installed side by side for the same user. Each home has its own `config.json`, `.env`, PID file and
logs. Ports are **not** assigned automatically: every home reads `gateway.host`/`gateway.port` from
its own config, so set a distinct port in each config before running more than one instance. On
macOS and Linux the default `~/.mole` uses the unhashed `io.mole.agent` / `mole` name and other
homes append an 8-character path hash; on Windows every home, including the default, is
`Mole-<8hex>`.

## One binary, one port

The console and the `webchat` channel share a single HTTP server. It binds to loopback by default — `127.0.0.1:18790` (the `gateway.host` setting defaults to `localhost`). The bind address has no command-line override: to listen elsewhere, edit `gateway.host` and `gateway.port` in `~/.mole/config.json` and restart the process — `mole service restart` for an installed service, or relaunch `mole serve`.

The other channels are not served on that port: `telegram`, `qq`, `feishu` and `weixin` connect outbound to their platforms (long polling, WebSocket, or REST) and never listen on it.

## CLI

| Command | Description |
|---------|-------------|
| `mole` | Print help (does not start the gateway) |
| `mole serve [-d]` | Run the gateway + web console in the foreground; `-d` enables debug logging |
| `mole service <install\|reinstall\|uninstall\|start\|stop\|restart>` | Manage the user-level background service (see above) |
| `mole service status [--json]` | Show installed/running state, PID, console URL and health; never the token |
| `mole service logs [-f] [--lines 200]` | Tail the platform service log (launchd files, user journal, or `gateway.log`) |
| `mole service env refresh [--dry-run] [--restart]` | Import shell `PATH`/locale/proxy/TLS settings into `.env`, optionally restarting |
| `mole version` | Print version/build information |

(`help` is a Cobra built-in and is not listed above; `completion` is disabled.) Everything else — models, channels, tools — is configured from the console or in `config.json`.

## Configuration

Settings live in `~/.mole/config.json` (the directory is overridable with `$MOLE_HOME`, the file path with `$MOLE_CONFIG`) and secrets live in `~/.mole/.env` next to it. The web console edits both.

A secret field in `config.json` is a `$VAR` reference such as `"api_keys": ["$DEEPSEEK_API_KEY"]` or `"token": "$DASHBOARD_TOKEN"`; the value lives in `~/.mole/.env` (mode 0600), which the gateway loads at startup. Only whole values are expanded, an undefined variable is a load error, `$$` means a literal `$`, and expansion is not recursive. Copy `config/env.example` to `~/.mole/.env` and fill it in; `config/config.example.json` shows the matching references. Changes to `.env` apply on the next start: restart `mole serve`, or run `mole service restart` when the agent is installed as a service.

**Upgrading from an older version:** the loader accepts only the current `version`, and rejects unknown config keys by naming them in its error — so an existing `config.json` needs the removed keys deleted by hand. The changes you are most likely to hit:

- `channel_list.pico` → `channel_list.webchat` (the key doubles as the channel type),
- `~/.mole/.security.yml` is gone: move its values into `~/.mole/.env` and reference them from `config.json` as `$VAR` (`enc://` values are rejected; plaintext in `.env` or a `file://` path are supported),
- the `MOLE_*` configuration variables (`MOLE_DASHBOARD_TOKEN`, `MOLE_KEY_PASSPHRASE`, `MOLE_LOG_LEVEL`, `MOLE_LOG_FILE`, the `MOLE_GATEWAY_*` and `MOLE_CHANNELS_*` variables) are no longer read. Only `MOLE_HOME`, `MOLE_CONFIG` and `MOLE_BUILTIN_SKILLS` remain; set the equivalent config fields instead.

Sessions recorded under the old channel prefix are no longer listed.

## Channels

`telegram`, `qq`, `feishu`, `weixin` and `webchat` (the console's own channel).

## Providers

The OpenAI-compatible family (OpenAI, OpenRouter, DeepSeek, Groq, Qwen, Moonshot, Zhipu, Volcengine, Mistral, Ollama, vLLM, LM Studio, and others) plus Anthropic, including the native Anthropic Messages protocol.

## License

MIT — see [LICENSE](LICENSE).
