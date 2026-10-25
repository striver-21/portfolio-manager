// Package domain holds the core types of the portfolio manager and the
// analytics that operate on them. It has no dependency on HTTP or storage.
package domain

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

// Sentinel errors returned by the store layer and mapped to HTTP status codes
// by the API layer.
var (
	// ErrNotFound is returned when a record does not exist or is not visible
	// to the requesting user.
	ErrNotFound = errors.New("not found")
	// ErrConflict is returned when a uniqueness constraint is violated.
	ErrConflict = errors.New("already exists")
)

// User is an account that owns portfolios.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Portfolio is a named collection of assets owned by one user.
type Portfolio struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Assets is populated when a single portfolio is fetched and is then
	// always present in JSON (an empty slice marshals as []). List endpoints
	// leave it nil, and omitzero drops the nil slice from the output.
	Assets []Asset `json:"assets,omitzero"`
}

// Asset is a holding in a portfolio. Monetary amounts and quantities are
// arbitrary-precision decimals, never floats.
type Asset struct {
	ID            string          `json:"id"`
	PortfolioID   string          `json:"portfolio_id"`
	Symbol        string          `json:"symbol"`
	Quantity      decimal.Decimal `json:"quantity"`
	PurchasePrice decimal.Decimal `json:"purchase_price"`
	CurrentPrice  decimal.Decimal `json:"current_price"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Cost is what was paid for the holding: purchase price times quantity.
func (a Asset) Cost() decimal.Decimal { return a.PurchasePrice.Mul(a.Quantity) }

// Value is the holding's current market value: current price times quantity.
func (a Asset) Value() decimal.Decimal { return a.CurrentPrice.Mul(a.Quantity) }

// Stats summarises the performance of a set of assets.
type Stats struct {
	TotalValue       decimal.Decimal            `json:"total_value"`
	TotalCost        decimal.Decimal            `json:"total_cost"`
	TotalGain        decimal.Decimal            `json:"total_gain"`
	TotalGainPercent decimal.Decimal            `json:"total_gain_percent"`
	AssetCount       int                        `json:"asset_count"`
	AssetBreakdown   map[string]decimal.Decimal `json:"asset_breakdown"`
}

// gainPercentPlaces is the number of decimal places kept for the percentage
// figure; the absolute totals are exact and never rounded.
const gainPercentPlaces = 4

// CalculateStats computes totals and a per-symbol value breakdown. Assets with
// the same symbol are summed. TotalGainPercent is zero when TotalCost is zero.
func CalculateStats(assets []Asset) Stats {
	stats := Stats{
		AssetBreakdown: make(map[string]decimal.Decimal, len(assets)),
	}
	for _, a := range assets {
		cost := a.Cost()
		value := a.Value()
		stats.TotalCost = stats.TotalCost.Add(cost)
		stats.TotalValue = stats.TotalValue.Add(value)
		stats.AssetCount++
		stats.AssetBreakdown[a.Symbol] = stats.AssetBreakdown[a.Symbol].Add(value)
	}
	stats.TotalGain = stats.TotalValue.Sub(stats.TotalCost)
	if stats.TotalCost.IsPositive() {
		stats.TotalGainPercent = stats.TotalGain.
			Div(stats.TotalCost).
			Mul(decimal.NewFromInt(100)).
			Round(gainPercentPlaces)
	}
	return stats
}
