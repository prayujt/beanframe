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

For example, `COMPANY_NAME="Example Company"` displays **Example Company Beanframe**.
Leaving `COMPANY_NAME` empty displays **Beanframe**.
The workspace title after login comes from the ledger's `option "title"`.
Company and provider names are independent: an organization can use any compatible
OIDC provider without changing the application code.

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
