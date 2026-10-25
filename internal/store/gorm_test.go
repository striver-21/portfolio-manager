package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/striver-21/portfolio-manager/internal/domain"
	"github.com/striver-21/portfolio-manager/internal/store"
	"github.com/striver-21/portfolio-manager/internal/store/storetest"
)

func newStore(t *testing.T) *store.Gorm {
	t.Helper()
	return store.NewGorm(storetest.Open(t))
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func seedUser(t *testing.T, s store.Store, email string) *domain.User {
	t.Helper()
	u := &domain.User{Email: email, PasswordHash: "hash"}
	require.NoError(t, s.CreateUser(context.Background(), u))
	require.NotEmpty(t, u.ID)
	return u
}

func seedPortfolio(t *testing.T, s store.Store, userID, name string) *domain.Portfolio {
	t.Helper()
	p := &domain.Portfolio{UserID: userID, Name: name, Description: "desc"}
	require.NoError(t, s.CreatePortfolio(context.Background(), p))
	return p
}

func TestCreateUserDuplicateEmailIsConflict(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	seedUser(t, s, "a@example.com")

	err := s.CreateUser(ctx, &domain.User{Email: "a@example.com", PasswordHash: "x"})
	assert.ErrorIs(t, err, domain.ErrConflict)

	_, err = s.UserByEmail(ctx, "nobody@example.com")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	u, err := s.UserByEmail(ctx, "a@example.com")
	require.NoError(t, err)
	assert.Equal(t, "a@example.com", u.Email)
	assert.False(t, u.CreatedAt.IsZero())
}

func TestPortfolioIsScopedToOwner(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	owner := seedUser(t, s, "owner@example.com")
	other := seedUser(t, s, "other@example.com")
	p := seedPortfolio(t, s, owner.ID, "Retirement")

	got, err := s.Portfolio(ctx, p.ID, owner.ID)
	require.NoError(t, err)
	assert.Equal(t, "Retirement", got.Name)
	assert.NotNil(t, got.Assets, "assets must be an empty slice, not nil")
	assert.Empty(t, got.Assets)

	_, err = s.Portfolio(ctx, p.ID, other.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	assert.ErrorIs(t, s.UpdatePortfolio(ctx, &domain.Portfolio{ID: p.ID, UserID: other.ID, Name: "x"}), domain.ErrNotFound)
	assert.ErrorIs(t, s.DeletePortfolio(ctx, p.ID, other.ID), domain.ErrNotFound)

	// The failed delete by the other user must not have removed anything.
	_, err = s.Portfolio(ctx, p.ID, owner.ID)
	require.NoError(t, err)
}

func TestUpdatePortfolioClearsDescription(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	u := seedUser(t, s, "a@example.com")
	p := seedPortfolio(t, s, u.ID, "Growth")
	created := p.UpdatedAt

	time.Sleep(2 * time.Millisecond)
	p.Description = ""
	p.Name = "Growth v2"
	require.NoError(t, s.UpdatePortfolio(ctx, p))

	got, err := s.Portfolio(ctx, p.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "Growth v2", got.Name)
	assert.Equal(t, "", got.Description)
	assert.True(t, got.UpdatedAt.After(created), "updated_at must advance")
}

func TestPortfoliosPagination(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	u := seedUser(t, s, "a@example.com")
	seedUser(t, s, "b@example.com") // noise from another user must not appear
	for i := 0; i < 5; i++ {
		seedPortfolio(t, s, u.ID, "P"+string(rune('A'+i)))
	}

	items, total, err := s.Portfolios(ctx, u.ID, store.Page{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.EqualValues(t, 5, total)
	assert.Len(t, items, 2)
	assert.Nil(t, items[0].Assets, "list does not load assets")

	items, _, err = s.Portfolios(ctx, u.ID, store.Page{Limit: 2, Offset: 4})
	require.NoError(t, err)
	assert.Len(t, items, 1)

	items, total, err = s.Portfolios(ctx, "no-such-user", store.Page{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, total)
	assert.NotNil(t, items)
	assert.Empty(t, items)
}

func TestPageNormalize(t *testing.T) {
	assert.Equal(t, store.Page{Limit: store.DefaultLimit, Offset: 0}, store.Page{}.Normalize())
	assert.Equal(t, store.Page{Limit: store.MaxLimit, Offset: 0}, store.Page{Limit: 10_000, Offset: -3}.Normalize())
	assert.Equal(t, store.Page{Limit: 7, Offset: 3}, store.Page{Limit: 7, Offset: 3}.Normalize())
}

func TestAssetsRoundTripDecimalsExactly(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	u := seedUser(t, s, "a@example.com")
	p := seedPortfolio(t, s, u.ID, "Crypto")

	a := &domain.Asset{
		PortfolioID:   p.ID,
		Symbol:        "BTC",
		Quantity:      dec("0.12345678"),
		PurchasePrice: dec("61234.56789012"),
		CurrentPrice:  dec("0.1"),
	}
	require.NoError(t, s.CreateAsset(ctx, a))

	got, err := s.Asset(ctx, a.ID, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "0.12345678", got.Quantity.String())
	assert.Equal(t, "61234.56789012", got.PurchasePrice.String())
	assert.Equal(t, "0.1", got.CurrentPrice.String())

	_, err = s.Asset(ctx, a.ID, "other-portfolio")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	got.CurrentPrice = dec("70000")
	require.NoError(t, s.UpdateAsset(ctx, got))
	again, err := s.Asset(ctx, a.ID, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "70000", again.CurrentPrice.String())

	assert.ErrorIs(t, s.UpdateAsset(ctx, &domain.Asset{ID: "missing", PortfolioID: p.ID}), domain.ErrNotFound)
	assert.ErrorIs(t, s.DeleteAsset(ctx, "missing", p.ID), domain.ErrNotFound)
	require.NoError(t, s.DeleteAsset(ctx, a.ID, p.ID))
	assert.ErrorIs(t, s.DeleteAsset(ctx, a.ID, p.ID), domain.ErrNotFound)
}

func TestDeletePortfolioRemovesAssets(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	u := seedUser(t, s, "a@example.com")
	p := seedPortfolio(t, s, u.ID, "Tech")
	a := &domain.Asset{PortfolioID: p.ID, Symbol: "AAPL", Quantity: dec("1"), PurchasePrice: dec("1"), CurrentPrice: dec("1")}
	require.NoError(t, s.CreateAsset(ctx, a))

	require.NoError(t, s.DeletePortfolio(ctx, p.ID, u.ID))
	assert.ErrorIs(t, s.DeletePortfolio(ctx, p.ID, u.ID), domain.ErrNotFound)
	_, err := s.Asset(ctx, a.ID, p.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestPortfolioLoadsAssetsInCreationOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	u := seedUser(t, s, "a@example.com")
	p := seedPortfolio(t, s, u.ID, "Ordered")
	for _, sym := range []string{"C", "A", "B"} {
		require.NoError(t, s.CreateAsset(ctx, &domain.Asset{
			PortfolioID: p.ID, Symbol: sym, Quantity: dec("1"), PurchasePrice: dec("1"), CurrentPrice: dec("1"),
		}))
		time.Sleep(2 * time.Millisecond)
	}
	got, err := s.Portfolio(ctx, p.ID, u.ID)
	require.NoError(t, err)
	require.Len(t, got.Assets, 3)
	assert.Equal(t, []string{"C", "A", "B"}, []string{got.Assets[0].Symbol, got.Assets[1].Symbol, got.Assets[2].Symbol})
}

func TestPingAndContextCancellation(t *testing.T) {
	s := newStore(t)
	require.NoError(t, s.Ping(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.UserByEmail(ctx, "a@example.com")
	assert.ErrorIs(t, err, context.Canceled)
}
