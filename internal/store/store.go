// Package store defines the persistence boundary of the service and provides
// a GORM-backed implementation plus an embedded SQL migration runner.
package store

import (
	"context"

	"github.com/striver-21/portfolio-manager/internal/domain"
)

// Pagination limits used by Page.Normalize.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Page describes one slice of a list result.
type Page struct {
	Limit  int
	Offset int
}

// Normalize clamps the page to sane bounds.
func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = DefaultLimit
	}
	if p.Limit > MaxLimit {
		p.Limit = MaxLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

// Store is everything the HTTP layer needs from persistence. Every method
// honours ctx cancellation. "Not visible to this user" is reported as
// domain.ErrNotFound, never as an empty result with a nil error.
type Store interface {
	CreateUser(ctx context.Context, u *domain.User) error // domain.ErrConflict on duplicate email
	UserByEmail(ctx context.Context, email string) (*domain.User, error)

	CreatePortfolio(ctx context.Context, p *domain.Portfolio) error
	// Portfolio returns one portfolio with its assets, scoped to userID.
	Portfolio(ctx context.Context, id, userID string) (*domain.Portfolio, error)
	// Portfolios returns one page of the user's portfolios (without assets)
	// and the total count.
	Portfolios(ctx context.Context, userID string, page Page) ([]domain.Portfolio, int64, error)
	UpdatePortfolio(ctx context.Context, p *domain.Portfolio) error
	DeletePortfolio(ctx context.Context, id, userID string) error

	CreateAsset(ctx context.Context, a *domain.Asset) error
	Asset(ctx context.Context, id, portfolioID string) (*domain.Asset, error)
	UpdateAsset(ctx context.Context, a *domain.Asset) error
	DeleteAsset(ctx context.Context, id, portfolioID string) error

	// Ping checks that the backing database is reachable.
	Ping(ctx context.Context) error
}
