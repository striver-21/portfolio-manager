package domain

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestCalculateStats(t *testing.T) {
	assets := []Asset{
		{Symbol: "AAPL", Quantity: dec("10"), PurchasePrice: dec("100"), CurrentPrice: dec("150")},
		{Symbol: "GOOGL", Quantity: dec("5"), PurchasePrice: dec("200"), CurrentPrice: dec("250")},
	}

	stats := CalculateStats(assets)

	assert.Equal(t, 2, stats.AssetCount)
	assert.True(t, dec("2750").Equal(stats.TotalValue), "value %s", stats.TotalValue)
	assert.True(t, dec("2000").Equal(stats.TotalCost), "cost %s", stats.TotalCost)
	assert.True(t, dec("750").Equal(stats.TotalGain), "gain %s", stats.TotalGain)
	assert.True(t, dec("37.5").Equal(stats.TotalGainPercent), "pct %s", stats.TotalGainPercent)
	require.Len(t, stats.AssetBreakdown, 2)
	assert.True(t, dec("1500").Equal(stats.AssetBreakdown["AAPL"]))
	assert.True(t, dec("1250").Equal(stats.AssetBreakdown["GOOGL"]))
}

func TestCalculateStatsIsExactWithFractions(t *testing.T) {
	// 0.1 + 0.2 is the classic float trap; decimals must sum to exactly 0.3.
	assets := []Asset{
		{Symbol: "A", Quantity: dec("1"), PurchasePrice: dec("0.1"), CurrentPrice: dec("0.1")},
		{Symbol: "B", Quantity: dec("1"), PurchasePrice: dec("0.2"), CurrentPrice: dec("0.2")},
	}
	stats := CalculateStats(assets)
	assert.Equal(t, "0.3", stats.TotalValue.String())
	assert.True(t, stats.TotalGain.IsZero())
	assert.True(t, stats.TotalGainPercent.IsZero())
}

func TestCalculateStatsMergesDuplicateSymbols(t *testing.T) {
	assets := []Asset{
		{Symbol: "AAPL", Quantity: dec("1"), PurchasePrice: dec("10"), CurrentPrice: dec("20")},
		{Symbol: "AAPL", Quantity: dec("2"), PurchasePrice: dec("10"), CurrentPrice: dec("20")},
	}
	stats := CalculateStats(assets)
	assert.Equal(t, 2, stats.AssetCount)
	require.Len(t, stats.AssetBreakdown, 1)
	assert.True(t, dec("60").Equal(stats.AssetBreakdown["AAPL"]))
	assert.True(t, dec("100").Equal(stats.TotalGainPercent))
}

func TestCalculateStatsEmpty(t *testing.T) {
	stats := CalculateStats(nil)
	assert.Equal(t, 0, stats.AssetCount)
	assert.True(t, stats.TotalValue.IsZero())
	assert.True(t, stats.TotalGainPercent.IsZero())
	assert.NotNil(t, stats.AssetBreakdown, "breakdown must marshal as {} not null")
}
