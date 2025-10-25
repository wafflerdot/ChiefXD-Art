# ChiefXD Art Discord Bot (Go)

A Discord bot written in Go that analyses images using Sightengine, with standard and advanced outputs, a role-based permissions system with DB or JSON persistence, and Cloud Run–friendly health checks.

## Features
- Image analysis via Sightengine
  - Standard summary with configurable thresholds
  - Advanced mode with category/subcategory score breakdown
  - AI-only analysis command
- Role-based permissions per guild (owner/admin override)
  - DB-backed (PostgreSQL/MySQL) or JSON-backed whitelist of moderator roles per server
  - Manage via `/permissions` command (add/remove/list)
- Slash commands registered globally by default or per-server when `GUILD_ID` is set
- Health endpoints for Cloud Run (`/`, `/healthz`)
- Discord Rich Presence: Watching ChiefXD

## Slash Commands
- `/analyse image_url:<URL> [advanced:boolean]`
  - Standard: returns Allowed and core scores
  - Advanced: adds per-category scores (explicit/suggestive nudity, offensive, AI usage)
- `/ai image_url:<URL>`
  - AI-only check using Sightengine genAI model
- `/thresholds`
  - Displays current detection thresholds
- `/ping`
  - Round-trip time and API latency
- `/help`
  - Lists all available commands with descriptions
- `/permissions <add|remove|list>`
  - `add role:<Role>`: Add a role to the whitelist
  - `remove role:<Role>`: Remove a role from the whitelist
  - `list`: Show current whitelisted moderator roles

Restricted commands: `/analyse`, `/ai`, `/thresholds`.
- Access permitted if the user is:
  - the configured owner (see `OWNER_ID`), or
  - an admin (Administrator/Manage Server), or
  - has at least one role on the server whitelist.

## Configuration
Set the following environment variables (can be in a local `.env` file loaded at startup):

- `BOT_TOKEN` (required): Discord bot token
- `SIGHTENGINE_USER` (required): Sightengine API user
- `SIGHTENGINE_SECRET` (required): Sightengine API secret
- `OWNER_ID` (recommended): Your Discord user ID (owner override)
- `GUILD_ID` (optional): If set, commands register for that guild; otherwise they register globally
- `PORT` (optional): HTTP port for health server (default: `8080`)

Permissions storage:
- DB-backed (recommended for production):
  - `PERMS_DSN`: database connection string
  - `PERMS_DIALECT`: `postgres` (default) or `mysql`

  Examples:
  - Postgres: `PERMS_DSN=postgres://user:pass@host:5432/db?sslmode=disable`
  - MySQL:    `PERMS_DSN=user:pass@tcp(host:3306)/db?parseTime=true`

  On first start, the bot will create a table named `permissions` with primary key `(guild_id, role_id)`.

- JSON-backed (fallback for dev/small deployments):
  - `PERMS_FILE` (default: `permissions.json`)

Thresholds are defined in `analysis.go` - default values:
- Nudity (Suggestive): 0.75
- Nudity (Explicit): 0.25
- Offensive: 0.25
- AI Generated: 0.60

## Run locally
1) Ensure Go is installed (Go 1.21+ recommended).
2) Create a `.env` file (see Configuration).
3) Run the bot:

```bash
go run ./...
```

Or build first:

```bash
go build -o chiefxdart .
./chiefxdart
```

The process starts an HTTP server for health checks and the Discord gateway session.

## Registering Commands: Global vs Guild
- Development (fast): set `GUILD_ID` to your dev guild. Commands appear instantly.
- Production (global): leave `GUILD_ID` empty. Commands may take up to ~1 hour to appear across all guilds.

The bot logs every created command and later lists the commands that Discord reports for the chosen scope.

## Cloud Run
- The bot starts an HTTP server on `PORT` (default `8080`) and responds `ok` on `/` and `/healthz`.
- Use the provided `Dockerfile` to containerise.
- For persistent role whitelists across revisions, point `PERMS_FILE` to a writable, persistent path (for example, a mounted volume). Ephemeral container filesystems may not persist across deployments.
- For DB-backed permissions, use a managed Postgres/MySQL (e.g., Cloud SQL) and provide `PERMS_DSN`

## Troubleshooting
- Commands don’t show up
  - If `GUILD_ID` is blank, commands are global and can take ~1 hour to propagate
  - Ensure the bot is invited with the `applications.commands` scope
  - Check Cloud Run logs for “created command:” and the debug list of stored commands
- `Unknown interaction` errors
  - Discord requires responding within 3s; the bot defers interactions and then edits. If you still see this, interactions may have expired (e.g., very long processing or client/network issues)
- Health check fails / container not listening
  - The bot starts an HTTP server on `PORT` with `/` and `/healthz`; ensure `PORT` is set by the platform (Cloud Run sets `PORT` automatically)
- Sightengine errors
  - Confirm `SIGHTENGINE_USER` and `SIGHTENGINE_SECRET` are set and valid
- DB errors
  - Ensure `PERMS_DSN` is correct and reachable; check logs for table creation and queries

## Project Structure
- `main.go`: bootstrap, persistence wiring (DB or JSON), session, handlers, registration, shutdown
- `http_server.go`: minimal HTTP health server
- `handlers.go`: slash command handlers
- `register.go`: command registration
- `permissions.go`: role allowlist store + DB/JSON persistence
- `analysis.go`: thresholds and scoring logic
- `sightengine.go`: API calls to Sightengine
- `rich_presence.go`: Discord rich presence
- `Dockerfile`: container build

## Choosing a database
- PostgreSQL is generally preferred for consistency and richer SQL features
- MySQL is also supported and may be preferable in environments where MySQL expertise/tooling already exists

## Security
- Use secret managers or Cloud Run secrets for credentials
- Keep secrets out of version control; use Cloud Run secrets or environment variables
- If a token has been exposed, rotate it immediately
## Licence
- All rights reserved
