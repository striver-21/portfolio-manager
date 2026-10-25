# Portfolio Manager API

A small REST API for tracking investment portfolios, written in Go. Gin for
routing, GORM over PostgreSQL for persistence, HS256 JWTs for authentication.
It is a portfolio project: deliberately narrow in scope, with the plumbing a
real service needs done properly.

## Architecture

```
          HTTP                 internal/httpapi                   internal/store
client ───────▶ gin router ──▶ middleware ──▶ handlers ──▶ store.Store ──▶ GORM ──▶ PostgreSQL
                              request-id      bind/validate   (interface)   (impl)
                              slog access log  map errors
                              recovery         to status codes      ▲
                              JWT auth                              │ embedded SQL
                              rate limit (auth)                     │ migrations
                                     │
                                     ▼
                              internal/domain  (User, Portfolio, Asset, Stats; ErrNotFound/ErrConflict)
                              internal/auth    (bcrypt, JWT issue/verify)
```

Dependency direction is strictly inward: `httpapi` depends on `store.Store`
(an interface) and `domain`; `store` depends on `domain`; `domain` depends on
nothing in the repo. `cmd/server/main.go` wires the concrete pieces together.

## Running it

### With Docker Compose

```bash
docker compose up --build
```

Starts Postgres 16 (with a health check), waits for it, applies migrations
(`-migrate`) and serves on `http://localhost:8080`. A development `JWT_SECRET`
is baked into `docker-compose.yml`; override it with
`JWT_SECRET=... docker compose up`.

### Locally against your own Postgres

```bash
createdb portfolio_manager
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/portfolio_manager?sslmode=disable"
export JWT_SECRET="$(openssl rand -base64 48)"

go run ./cmd/server -migrate-only   # apply schema, exit
go run ./cmd/server                 # serve
```

### Configuration

Everything comes from the environment (see `.env.example`):

| Variable       | Required | Default             | Notes                                      |
|----------------|----------|---------------------|--------------------------------------------|
| `DATABASE_URL` | yes      |                     | Postgres DSN (URL or key=value form)       |
| `JWT_SECRET`   | yes      |                     | HS256 key, at least 32 bytes               |
| `PORT`         | no       | `8080`              |                                            |
| `JWT_ISSUER`   | no       | `portfolio-manager` | `iss` claim, checked on verify             |
| `TOKEN_TTL`    | no       | `24h`               | Go duration                                |
| `LOG_LEVEL`    | no       | `info`              | `debug`, `info`, `warn`, `error`           |
| `GIN_MODE`     | no       | `release`           | Read by Gin; forced to release when unset  |

The process refuses to start if `DATABASE_URL` or `JWT_SECRET` is missing, and
reports every configuration problem in one message.

Flags: `-migrate` applies pending migrations then serves; `-migrate-only`
applies them and exits.

## API

The full contract is in [`api/openapi.yaml`](api/openapi.yaml). Summary:

| Method | Path                                         | Auth | Notes                                     |
|--------|----------------------------------------------|------|-------------------------------------------|
| GET    | `/health`                                    | no   | 200 if the DB answers a ping, else 503    |
| POST   | `/api/auth/register`                         | no   | 201; 409 on duplicate email; rate limited |
| POST   | `/api/auth/login`                            | no   | 200 with JWT; 401; rate limited           |
| GET    | `/api/portfolios?limit=&offset=`             | yes  | Paginated, newest first, no assets        |
| POST   | `/api/portfolios`                            | yes  | 201                                       |
| GET    | `/api/portfolios/{id}`                       | yes  | Includes `assets`                         |
| PUT    | `/api/portfolios/{id}`                       | yes  | Partial; `"description": ""` clears       |
| DELETE | `/api/portfolios/{id}`                       | yes  | 204; 404 if nothing was deleted           |
| GET    | `/api/portfolios/{id}/stats`                 | yes  | Exact decimal totals                      |
| POST   | `/api/portfolios/{id}/assets`                | yes  | 201                                       |
| PUT    | `/api/portfolios/{id}/assets/{asset_id}`     | yes  | Partial                                   |
| DELETE | `/api/portfolios/{id}/assets/{asset_id}`     | yes  | 204 / 404                                 |

Errors are always `{"error": "...", "details": [...]}`. A portfolio that
belongs to someone else is a 404, not a 403, so ids cannot be probed.

### curl walkthrough

```bash
BASE=http://localhost:8080

# 1. Register and log in
curl -s -X POST $BASE/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"ada@example.com","password":"correct-horse-battery"}'
# {"email":"ada@example.com","id":"5d1b..."}

TOKEN=$(curl -s -X POST $BASE/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"ada@example.com","password":"correct-horse-battery"}' | jq -r .token)

# 2. Create a portfolio
PF=$(curl -s -X POST $BASE/api/portfolios \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Long term","description":"Index funds and a little tech"}' | jq -r .id)

# 3. Add holdings. Amounts are decimal strings (numbers are accepted too).
curl -s -X POST $BASE/api/portfolios/$PF/assets \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"symbol":"AAPL","quantity":"10","purchase_price":"100","current_price":"150"}'
curl -s -X POST $BASE/api/portfolios/$PF/assets \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"symbol":"GOOGL","quantity":"5","purchase_price":"200","current_price":"250"}'

# 4. Stats
curl -s $BASE/api/portfolios/$PF/stats -H "Authorization: Bearer $TOKEN"
# {"total_value":"2750","total_cost":"2000","total_gain":"750",
#  "total_gain_percent":"37.5","asset_count":2,
#  "asset_breakdown":{"AAPL":"1500","GOOGL":"1250"}}

# 5. Clear the description, then list
curl -s -X PUT $BASE/api/portfolios/$PF \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"description":""}'
curl -s "$BASE/api/portfolios?limit=10" -H "Authorization: Bearer $TOKEN"

# 6. Delete (204), then confirm it is gone (404)
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE $BASE/api/portfolios/$PF -H "Authorization: Bearer $TOKEN"
curl -s -o /dev/null -w '%{http_code}\n' $BASE/api/portfolios/$PF -H "Authorization: Bearer $TOKEN"
```

## Design notes

- **Money is decimal.** Quantities and prices are `shopspring/decimal`, stored
  as `NUMERIC(24,8)`, and rendered as JSON strings. Analytics never touch
  `float64`. Inputs are validated to fit the column (positive, up to 8 decimal
  places, below 10^16).
- **Storage behind an interface.** Handlers only see `store.Store`. The GORM
  implementation passes `ctx` to every query, translates driver errors to
  `domain.ErrNotFound` / `domain.ErrConflict`, and checks `RowsAffected` so a
  delete or update of a missing row is a 404, not a silent 200.
- **No check-then-insert.** Registration relies on the unique index and maps
  the unique-violation error to 409, so two concurrent registrations of the
  same email cannot both succeed or produce a 500.
- **Explicit SQL migrations**, embedded with `embed` and applied by a small
  runner (`internal/store/migrate.go`) that records versions in
  `schema_migrations` and runs each file in a transaction. No `AutoMigrate`
  for application tables.
- **JWT hygiene.** Typed claims (`jwt.RegisteredClaims` + `uid`), signing
  method pinned to HS256 via `jwt.WithValidMethods`, issuer and expiry
  required, secret must be at least 32 bytes and come from the environment.
- **Login hardening.** bcrypt (cost 10); unknown emails still pay a bcrypt
  compare so timing does not reveal account existence; `/api/auth/*` is
  rate limited per client IP with an in-memory token bucket (10/min, burst 10).
- **Operational plumbing.** `log/slog` JSON logs with a per-request
  `X-Request-ID`, `http.Server` timeouts, `signal.NotifyContext` graceful
  shutdown, Gin in release mode, distroless non-root container.

Known limits, on purpose: the rate limiter is per process (use the edge or
Redis across replicas); proxies are not trusted, so behind a load balancer
`ClientIP()` is the balancer until trusted CIDRs are configured; there is no
token revocation or refresh.

## Testing

```bash
go test -race -count=1 -cover ./...
```

The suite needs no Docker and no running database: it uses the pure-Go
`github.com/glebarez/sqlite` driver with a fresh in-memory database per test,
migrated by the same runner that migrates Postgres. One test
(`TestPostgresMigrationsApply`) exercises the real Postgres migrations and is
skipped unless `TEST_DATABASE_URL` is set; CI provides a Postgres service for it.

Measured coverage (`go tool cover -func`, all packages): **77%** of
statements. `cmd/server/main.go` is the untested part; the packages under
`internal/` range from 80% (`store`) through 85% (`httpapi`) and 97% (`auth`)
to 100% (`config`, `domain`, `ratelimit`).

CI (`.github/workflows/ci.yml`) runs build, vet, gofmt, the race-enabled
tests, golangci-lint and govulncheck.

## Layout

```
cmd/server/            main: config, wiring, migrations flag, graceful shutdown
internal/auth/         bcrypt helpers, JWT TokenManager
internal/config/       environment parsing
internal/domain/       core types, sentinel errors, stats calculation
internal/httpapi/      gin router, middleware, handlers, HTTP tests
internal/ratelimit/    in-memory token bucket
internal/store/        Store interface, GORM implementation, migration runner,
                       migrations/{postgres,sqlite}/*.sql, storetest helper
api/openapi.yaml       OpenAPI 3 description of the API
```

## License

MIT. See [LICENSE](LICENSE).
