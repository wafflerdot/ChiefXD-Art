# ChiefXD Art Discord Bot (Go)

A Discord bot written in Go that analyses images using Sightengine, with standard and advanced outputs, a role-based permissions system with DB or JSON persistence, and Cloud Run–friendly health checks.

## Features
- Sightengine-powered image analysis
  - Standard analysis: returns normalised scores and an `Allowed` verdict computed using the guild's thresholds (fallback to defaults when none set).
  - Advanced analysis: detailed per-category and per-subcategory numeric scores (explicit vs suggestive nudity, offensive symbols, AI usage). Note: advanced mode does not compute or return `Allowed`.
  - AI-only analysis: checks only AI-generation score (uses guild thresholds for the allowed verdict).
- Slash commands with role-based access control
  - `/permissions` to add/remove/list moderator roles for each guild
  - `/thresholds` subcommands to view, set, reset, and view history of thresholds per guild
  - `/analyse` and `/ai` are restricted to allowed roles, admins, or configured owner
  - `/ping` and `/help` for diagnostics and documentation
- Storage options
  - DB-backed (Postgres or MySQL) — recommended for production (permissions + per-guild thresholds + history)
  - JSON-backed local files — convenient for development
- Cloud Run friendly: health endpoints, PORT usage, containerised via `Dockerfile`

## Slash Commands
- `/analyse image_url:<URL> [advanced:boolean]`
  - If `advanced=false` (default): the bot uses the guild thresholds to determine `Allowed` and lists the core scores (Nudity Explicit, Nudity Suggestive, Offensive, AI Generated).
  - If `advanced=true`: the bot returns a full score breakdown (category → subcategory → percent). Advanced output does NOT include an `Allowed` verdict.
- `/ai image_url:<URL>`
  - Runs only the AI (genAI) model and returns the AI score and an `Allowed` verdict computed via the guild's AI threshold.
- `/thresholds` (subcommands)
  - `/thresholds list` — shows the current thresholds for the server (guild-scoped values)
  - `/thresholds set name:<NuditySuggestive|NudityExplicit|Offensive|AIGenerated> value:<0.00–1.00 or percent>` — owner/admin only; stores the threshold for the current guild
  - `/thresholds reset name:<NuditySuggestive|NudityExplicit|Offensive|AIGenerated|all>` — owner/admin only; resets one or all thresholds to defaults for this guild
  - `/thresholds history [limit] [threshold]` — shows recent threshold changes for this guild; `threshold` can be filtered via a dropdown with the canonical choices (NuditySuggestive, NudityExplicit, Offensive, AIGenerated)
- `/permissions <add|remove|list>`
  - `add role:<Role>` — add role to guild whitelist (owner/admin only)
  - `remove role:<Role>` — remove role from guild whitelist
  - `list` — show roles allowed to use restricted commands; roles are displayed as mentions (`<@&ROLEID>`) separated by commas
- `/ping` — returns bot response time and API latency in an embed
- `/help` — detailed help embed including the thresholds subcommands and notes

Restricted commands: `/analyse`, `/ai`, `/permissions`, `/thresholds` (set/reset/history should be owner/admin-only; list/history view permitted to allowed roles and admins).

## Threshold Behaviour
- Each guild may have its own thresholds. The decision whether an image is Allowed is made by comparing the scores to the guild's thresholds.
- Fallback order when computing thresholds: guild thresholds → global thresholds table (if configured) → hard-coded defaults in code.
- Default threshold values (defined in `analysis.go`):
  - Nudity (Suggestive): 0.75
  - Nudity (Explicit): 0.25
  - Offensive: 0.25
  - AI Generated: 0.60
- Advanced mode returns raw sub-scores and does NOT compute Allowed — use standard/AI-only to get verdicts.

## Permissions and storage
- Permission storage options:
  - DB-backed (recommended): `PERMS_DSN` (connection string) + `PERMS_DIALECT` (`postgres` or `mysql`). The bot creates necessary tables for permissions, thresholds, and history.
  - JSON-backed (dev): `PERMS_FILE` (defaults to `permissions.json`) for local, simple storage.
- The permissions store controls which roles can use restricted commands. Owner (`OWNER_ID`) and server admins retain override access.
- Role mentions returned by the bot are formatted as Discord role mentions: `<@&ROLEID>` (so they appear as clickable mentions in Discord).

## Environment Variables / Configuration
Required for normal operation:
- `BOT_TOKEN` — Discord bot token
- `SIGHTENGINE_USER` — Sightengine API user
- `SIGHTENGINE_SECRET` — Sightengine API secret

Optional / recommended:
- `OWNER_ID` — Discord user id that acts as the owner override
- `GUILD_ID` — if set, the bot registers commands for this guild only (developer/dev-guild toggle); if empty the bot registers global commands (may take time to propagate)
- `PORT` — HTTP port for health endpoints (Cloud Run sets this automatically; default `8080`)

Permissions/DB:
- `PERMS_DIALECT` — `postgres` or `mysql` (default: `postgres`) when using DB
- `PERMS_DSN` — database connection string when using DB
- `PERMS_FILE` — path to JSON file for JSON-backed permissions storage (dev)

Notes about the dev toggle: leaving `GUILD_ID` empty registers commands globally (slow propagation). Setting `GUILD_ID` makes registration guild-scoped and instant — useful for development.

## Running locally
1. Ensure Go is installed (Go 1.21+ recommended).
2. Create a `.env` file (or set environment variables) with required values.
3. Run:

```bash
go run ./...
```

Or build and run:

```bash
go build -o chiefxdart .
./chiefxdart
```

The process starts an HTTP server for health checks and the Discord gateway session.

## Command Registration
- Development (fast): set `GUILD_ID` to your dev guild. Commands appear instantly.
- Production (global): leave `GUILD_ID` empty. Commands may take up to ~1 hour to appear across all guilds.

## Docker / Cloud Run Deployment
- The container must listen on the `PORT` environment variable (Cloud Run sets `PORT` automatically). The project provides a `Dockerfile`.
- For Cloud Run:
  - Supply environment variables via Cloud Run console or Secret Manager.
  - If you use DB-backed permissions/thresholds, point `PERMS_DSN` at a Cloud SQL or managed DB instance and set `PERMS_DIALECT` accordingly.
  - Ensure the container actually listens on the exposed `PORT` or Cloud Run will fail the revision (the startup error will indicate a port/listen problem).
- If you prefer JSON-backed storage in Cloud Run, ensure the JSON file points to a persistent mount or storage location — the container filesystem is ephemeral across revisions.

## Troubleshooting
- Commands not appearing in Discord:
  - If `GUILD_ID` is empty commands are registered globally and can take ~1 hour to appear. For instant registration use a guild-scoped `GUILD_ID` during development.
  - Ensure the bot has `applications.commands` scope.
- `Unknown interaction` errors:
  - Interactions must be replied to or deferred within 3s. Handler code defers and then edits the response; if you still see this, check for extremely long processing times or network issues.
- Container startup/health check errors on Cloud Run:
  - Confirm your container listens on `PORT` and responds to `/healthz` promptly.
- Sightengine API errors:
  - Confirm `SIGHTENGINE_USER` and `SIGHTENGINE_SECRET` are set and valid.
- DB errors:
  - Verify `PERMS_DSN` is reachable and credentials are correct. The bot attempts to create required tables on startup.

## Project layout
- `main.go` — bootstrap + wiring
- `handlers.go` — command handlers
- `register.go` — command registration logic
- `analysis.go` — scoring logic
- `sightengine.go` — Sightengine API calls
- `permissions.go` — role whitelist store (DB/JSON)
- `thresholds.go` — per-guild thresholds and history, including stores
- `http_server.go` — health endpoints
- `rich_presence.go` — Discord Rich Presence configuration
- `Dockerfile` — container build

## Databases
- PostgreSQL is generally preferred for consistency and richer SQL features
- MySQL is also supported and may be preferable in environments where MySQL expertise/tooling already exists

## Security
- Use secret managers or Cloud Run secrets for credentials
- Keep secrets out of version control; use Cloud Run secrets or environment variables
- If a token has been exposed, rotate it immediately

## AI Disclaimer
- AI assistance was used in some capacity in this project.

## Licence
- All rights reserved
