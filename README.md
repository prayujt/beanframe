# Beanframe

A self-hosted accounting workspace with a web interface and MCP tools. Edit
plain-text ledgers, review reports, and connect an assistant to the same validated
read and write operations. One Go server serves the Svelte UI, API, and MCP endpoint.

Beanframe uses the [Beancount](https://github.com/beancount/beancount) accounting
engine and file format. It is an independent project.

## Features

- Journal, account balances, income statements, balance sheets, CSV exports,
  date filters, and charts.
- Transaction forms and a syntax-aware source editor with validation before every
  write, revision conflict detection, and local recovery history.
- Fourteen MCP tools for querying and editing the ledger, with separate read and
  write permissions and live change notifications.
- Configurable branding and OpenID Connect sign-in, with PKCE, server-side
  sessions, refresh tokens, and signed backchannel logout.
- Persistent ledger files mounted into a single container. No application database.
- Optional Discord, Slack, and JSON webhook notifications for committed ledger changes.

Reports preserve decimal amounts and cost lots. They use book cost in the selected
currency; they do not perform foreign-exchange conversion or market valuation.

## Run locally

Requires Go 1.25+, Python 3.12+, Node 24, and the pnpm version pinned in
`package.json`.

```sh
make setup
make test
make build
AUTH_DISABLED=true PYTHON="$PWD/.venv/bin/python" \
  LISTEN_ADDR=127.0.0.1:8080 ./bin/beanframe
```

This uses synthetic sample data. `AUTH_DISABLED=true` is only for local development
and tests; authentication is enabled by default. For frontend development, run
`pnpm dev:web` alongside the Go server.

`make proto` regenerates the Go and TypeScript bindings. `make check` runs Python
checks, Go race tests and vet, and frontend type checks. `make test` also builds
and runs browser tests. CI exercises the published container image.

## Configuration

Settings are loaded at server startup. Branding is returned by the public session
endpoint, so changing it requires restarting the server but no frontend rebuild.

| Variable | Default | Purpose |
| --- | --- | --- |
| `COMPANY_NAME` | Empty | Derives the application name as `<Company> Beanframe`; empty means `Beanframe` |
| `COMPANY_LOGO_URL` | Empty | Optional HTTP(S) image URL or root-relative company logo path |
| `OIDC_PROVIDER_NAME` | `OpenID Connect` | Label in “Continue with …”; does not change the actual provider |
| `PUBLIC_URL` | Required | Canonical HTTPS origin, without a path |
| `OIDC_ISSUER_URL` | Required | Browser provider's discovery issuer |
| `OIDC_CLIENT_ID` | Required | Registered browser client |
| `OIDC_CLIENT_SECRET` | Empty | Confidential client secret; optional for a public PKCE client |
| `MCP_ISSUER_URL` | Browser issuer | Issuer used for MCP access tokens |
| `SESSION_TTL` | `15m` | Local session validation window; accepts `1m`–`1h` |
| `LEDGER_PATH` | `testdata/ledger/main.beancount` | Root ledger file; use a persistent volume in production |
| `LEDGER_SEED_DIR` | Empty | Seed an empty ledger directory; never overwrites an existing ledger |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `WEB_DIR` | `apps/web/build` | Built static frontend directory |
| `PYTHON` | `python3` | Python executable with the engine package installed |
| `LOG_LEVEL` | `debug` | `debug`, `info`, `warn`, or `error` |
| `LOG_FORMAT` | `text` | `text` or `json` |
| `DISCORD_WEBHOOK_URL` | Empty | Optional native Discord webhook URL for rich transaction embeds |
| `SLACK_WEBHOOK_URL` | Empty | Optional Slack incoming webhook URL for rich Block Kit messages |
| `JSON_WEBHOOK_URL` | Empty | Optional HTTP(S) endpoint for structured transaction events |
| `WEBHOOK_EVENTS` | `created` | Events sent to configured webhooks: `created`, `transactions`, or `all` |

For example, `COMPANY_NAME="Example Company"` displays **Example Company Beanframe**.
Leaving `COMPANY_NAME` empty displays **Beanframe**.
The workspace title after login comes from the ledger's `option "title"`.
Company and provider names are independent: an organization can use any compatible
OIDC provider without changing the application code.

### Webhook notifications

Set any combination of `DISCORD_WEBHOOK_URL`, `SLACK_WEBHOOK_URL`, and
`JSON_WEBHOOK_URL` on the server, then restart it. Empty or unset URLs disable
that destination. Use Discord's normal webhook URL, without the `/slack` suffix;
Beanframe sends native embeds. Slack receives Block Kit messages with a plain
text fallback. Both rich formats show the payee, narration, date, and postings,
preserving decimal amounts and currencies. Long rich messages are truncated to
fit provider limits; the JSON event retains the complete transaction.

`WEBHOOK_EVENTS` selects which successful changes to send to every configured
destination:

| Value | Events |
| --- | --- |
| `created` (default) | `transaction.created` only |
| `transactions` | `transaction.created`, `transaction.updated`, `transaction.deleted` |
| `all` | All transaction events plus `account.opened`, `account.closed`, `ledger.file_written`, `ledger.restored` |

The default fires only after a new transaction is validated and committed
through the web transaction form or MCP `save_transaction` with no transaction
ID, regardless of its Beancount flag. Transactions must appear in the validated
ledger snapshot; writes to files outside the root's include graph emit a file
event instead. Account, source-file, and restore events use their corresponding
UI and MCP write operations. Failed writes, saves with
no changes, external file changes, and existing transactions at startup never
send notifications, even with `all` selected. Restart the server after changing
the event selection.

The generic endpoint receives an HTTP POST with `Content-Type: application/json`
and these fields:

| Field | Value |
| --- | --- |
| `type` | One of the event names above |
| `occurred_at` | UTC timestamp when the committed change was queued |
| `revision` | Committed ledger revision |
| `file` | Relative path of the affected ledger file |
| `transaction` | Transaction events only: complete snapshot transaction, including ID, date, flag, payee, narration, tags, links, postings, source, file, and line; deletes include the removed transaction |
| `account`, `date` | Account events only: account name and opening/closing date |

Posting amounts remain decimal strings, with units, cost, price, and book value
in the same shape as the ledger snapshot. Each destination has its own background
worker and a queue of 128 events, so a slow or unavailable endpoint does not
block ledger writes or the other destinations. Requests time out after five
seconds. Network failures, HTTP 429, and HTTP 5xx responses get up to three total
attempts, with backoff and `Retry-After` support capped at 30 seconds per wait.
Other unsuccessful responses, including redirects, are not retried. Delivery
failures are logged without webhook URLs or response bodies.

Delivery is best effort: queues are held in memory, pending events are lost on
shutdown, and a full destination queue drops new events with a warning. Retries
can produce duplicates; custom receivers can deduplicate on `revision` plus
`type`. Webhook configuration stays server-side and is not exposed by the public
session endpoint.

## Authentication

Register `PUBLIC_URL/auth/callback` as the exact browser redirect URI. Enable
`openid`, `profile`, `email`, and `offline_access`. The provider must support
S256 PKCE and a UserInfo endpoint. Configure the provider's application access
policy to admit only the intended users. Displayed group claims are informational;
the application grants authenticated browser users read and write access.

Refresh tokens stay on the server. Sessions periodically check UserInfo and
accept signed logout notifications at `PUBLIC_URL/auth/backchannel-logout`.
Local sign-out ends the application session without signing out of other apps.
Sessions are held in memory: run one replica, and expect sign-in after a restart.

Production at `beanframe.prayujt.com` uses the **Beanframe** Authentik application
with public client ID `beanframe` and issuer
`https://auth.prayujt.com/application/o/beanframe/` for both browser login and MCP.
`nimbus.yaml` omits `MCP_ISSUER_URL` so MCP inherits `OIDC_ISSUER_URL`, and omits
`OIDC_CLIENT_SECRET` because both flows use S256 PKCE. The shared application's
access policy controls who can use either interface; MCP writes still require
`beancount:write`. Configure MCP integrations with client ID `beanframe` and no
client secret. Existing integrations using `beanframe-mcp` must reconnect after
the deployment switches issuers.

Branch previews retain their separate `beanframe-preview` client and issuer in
`nimbus-preview.yaml`.

### MCP clients

Connect to `PUBLIC_URL/mcp`. OAuth resource metadata is served at
`/.well-known/oauth-protected-resource/mcp`. Register MCP clients with the
configured issuer; the server does not implement anonymous dynamic client
registration. Public clients must use authorization code flow with S256 PKCE.

The issuer must supply signed JWT access tokens with:

- The configured issuer, a subject, and a valid expiry.
- An audience containing the exact `PUBLIC_URL/mcp` URL.
- `openid` and `beancount:read` scopes; also `beancount:write` for mutations.

These scope names remain stable for compatibility with existing engine clients.
The provider's UserInfo endpoint must accept access tokens and return the same
subject. Opaque access tokens are unsupported. Every MCP request validates the
token and checks UserInfo; provider failures deny access.

Read tools: `validate_ledger`, `list_accounts`, `get_balances`,
`list_transactions`, `get_ledger_snapshot`, `get_report`, `read_ledger_file`, and
`get_ledger_history`. Write tools: `save_transaction`, `delete_transaction`,
`open_account`, `close_account`, `write_ledger_file`, and `restore_ledger_version`.
Mutations require the current `expected_revision` to avoid overwriting concurrent
changes. MCP clients can subscribe to `ledger://snapshot` updates.

## Containers

```sh
docker build -f apps/server/Dockerfile -t beanframe:dev .
docker run --rm -e AUTH_DISABLED=true -p 127.0.0.1:8080:8080 beanframe:dev
```

Published images are `docker.prayujt.com/beanframe:<commit>` and
`docker.prayujt.com/beanframe:latest`, for amd64 and arm64. The container runs as
UID/GID `10001:10001`. Mounted ledger files and their parent directories must be
readable and writable by that identity. `/healthz` is the readiness endpoint.

Only mount trusted ledger files. Existing source files must stay within the
ledger directory; escaping paths and symlinks are rejected. API writes cannot
change Python plugins or include directives. Recovery history is stored beside
the ledger in `.beancount-history`; back up the complete volume. Local recovery
history is not an off-server backup.

## Engine licensing

The bundled engine is Beancount 3.2.3, licensed GPL-2.0-only. Its license and
corresponding source accompany the container distribution. File extensions,
engine imports, and the existing RPC and OAuth identifiers retain their upstream
format names for interoperability.
