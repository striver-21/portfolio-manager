-- SQLite flavour of 0001_init.sql, used only by the test suite.
-- Decimal columns are TEXT so SQLite's NUMERIC affinity cannot coerce them to
-- floats; the driver's TIMESTAMP decltype is what makes it hand back time.Time.

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT      NOT NULL,
    password_hash TEXT      NOT NULL,
    created_at    TIMESTAMP NOT NULL,
    updated_at    TIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX idx_users_email ON users (email);

CREATE TABLE portfolios (
    id          TEXT PRIMARY KEY,
    user_id     TEXT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        TEXT      NOT NULL,
    description TEXT      NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);

CREATE INDEX idx_portfolios_user_id ON portfolios (user_id);

CREATE TABLE assets (
    id             TEXT PRIMARY KEY,
    portfolio_id   TEXT      NOT NULL REFERENCES portfolios (id) ON DELETE CASCADE,
    symbol         TEXT      NOT NULL,
    quantity       TEXT      NOT NULL,
    purchase_price TEXT      NOT NULL,
    current_price  TEXT      NOT NULL,
    created_at     TIMESTAMP NOT NULL,
    updated_at     TIMESTAMP NOT NULL
);

CREATE INDEX idx_assets_portfolio_id ON assets (portfolio_id);
