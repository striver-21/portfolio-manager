-- Initial schema. Monetary columns are NUMERIC so values survive the round trip
-- to github.com/shopspring/decimal without floating-point rounding.

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX idx_users_email ON users (email);

CREATE TABLE portfolios (
    id          TEXT PRIMARY KEY,
    user_id     TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_portfolios_user_id ON portfolios (user_id);

CREATE TABLE assets (
    id             TEXT PRIMARY KEY,
    portfolio_id   TEXT           NOT NULL REFERENCES portfolios (id) ON DELETE CASCADE,
    symbol         TEXT           NOT NULL,
    quantity       NUMERIC(24, 8) NOT NULL CHECK (quantity > 0),
    purchase_price NUMERIC(24, 8) NOT NULL CHECK (purchase_price > 0),
    current_price  NUMERIC(24, 8) NOT NULL CHECK (current_price > 0),
    created_at     TIMESTAMPTZ    NOT NULL,
    updated_at     TIMESTAMPTZ    NOT NULL
);

CREATE INDEX idx_assets_portfolio_id ON assets (portfolio_id);
