package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/striver-21/portfolio-manager/internal/domain"
)

// GormConfig returns the GORM settings shared by every dialect: UTC timestamps,
// driver errors translated to gorm sentinels, and GORM's own logger silenced in
// favour of the application's slog logger.
func GormConfig() *gorm.Config {
	return &gorm.Config{
		NowFunc:        func() time.Time { return time.Now().UTC() },
		TranslateError: true,
		Logger:         logger.Default.LogMode(logger.Silent),
	}
}

// OpenPostgres connects to Postgres, configures the pool and verifies the
// connection with a ping bounded by ctx.
func OpenPostgres(ctx context.Context, dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), GormConfig())
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

// Gorm implements Store on top of *gorm.DB.
type Gorm struct {
	db *gorm.DB
}

// NewGorm wraps an opened GORM connection.
func NewGorm(db *gorm.DB) *Gorm { return &Gorm{db: db} }

var _ Store = (*Gorm)(nil)

// translate maps GORM sentinels onto domain errors so callers never import gorm.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return domain.ErrConflict
	default:
		return err
	}
}

func newID() string { return uuid.NewString() }

// Ping implements Store.
func (s *Gorm) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// CreateUser implements Store.
func (s *Gorm) CreateUser(ctx context.Context, u *domain.User) error {
	if u.ID == "" {
		u.ID = newID()
	}
	return translate(s.db.WithContext(ctx).Create(u).Error)
}

// UserByEmail implements Store.
func (s *Gorm) UserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := s.db.WithContext(ctx).Where("email = ?", email).First(&u).Error
	if err != nil {
		return nil, translate(err)
	}
	return &u, nil
}

// CreatePortfolio implements Store.
func (s *Gorm) CreatePortfolio(ctx context.Context, p *domain.Portfolio) error {
	if p.ID == "" {
		p.ID = newID()
	}
	return translate(s.db.WithContext(ctx).Omit("Assets").Create(p).Error)
}

// Portfolio implements Store.
func (s *Gorm) Portfolio(ctx context.Context, id, userID string) (*domain.Portfolio, error) {
	var p domain.Portfolio
	err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Preload("Assets", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC, id ASC")
		}).
		First(&p).Error
	if err != nil {
		return nil, translate(err)
	}
	if p.Assets == nil {
		p.Assets = []domain.Asset{}
	}
	return &p, nil
}

// Portfolios implements Store.
func (s *Gorm) Portfolios(ctx context.Context, userID string, page Page) ([]domain.Portfolio, int64, error) {
	page = page.Normalize()
	q := s.db.WithContext(ctx).Model(&domain.Portfolio{}).Where("user_id = ?", userID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err)
	}
	items := make([]domain.Portfolio, 0, page.Limit)
	err := q.Order("created_at DESC, id DESC").
		Limit(page.Limit).
		Offset(page.Offset).
		Find(&items).Error
	if err != nil {
		return nil, 0, translate(err)
	}
	return items, total, nil
}

// UpdatePortfolio implements Store. Name and Description are written
// unconditionally, so an empty Description clears the column.
func (s *Gorm) UpdatePortfolio(ctx context.Context, p *domain.Portfolio) error {
	p.UpdatedAt = s.db.NowFunc()
	res := s.db.WithContext(ctx).
		Model(&domain.Portfolio{}).
		Where("id = ? AND user_id = ?", p.ID, p.UserID).
		Select("name", "description", "updated_at").
		Updates(p)
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeletePortfolio implements Store. Assets are removed in the same transaction
// so the result does not depend on the database enforcing ON DELETE CASCADE.
func (s *Gorm) DeletePortfolio(ctx context.Context, id, userID string) error {
	return translate(s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("portfolio_id = ?", id).Delete(&domain.Asset{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&domain.Portfolio{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound // rolls back the asset delete above
		}
		return nil
	}))
}

// CreateAsset implements Store.
func (s *Gorm) CreateAsset(ctx context.Context, a *domain.Asset) error {
	if a.ID == "" {
		a.ID = newID()
	}
	return translate(s.db.WithContext(ctx).Create(a).Error)
}

// Asset implements Store.
func (s *Gorm) Asset(ctx context.Context, id, portfolioID string) (*domain.Asset, error) {
	var a domain.Asset
	err := s.db.WithContext(ctx).Where("id = ? AND portfolio_id = ?", id, portfolioID).First(&a).Error
	if err != nil {
		return nil, translate(err)
	}
	return &a, nil
}

// UpdateAsset implements Store.
func (s *Gorm) UpdateAsset(ctx context.Context, a *domain.Asset) error {
	a.UpdatedAt = s.db.NowFunc()
	res := s.db.WithContext(ctx).
		Model(&domain.Asset{}).
		Where("id = ? AND portfolio_id = ?", a.ID, a.PortfolioID).
		Select("quantity", "purchase_price", "current_price", "updated_at").
		Updates(a)
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteAsset implements Store.
func (s *Gorm) DeleteAsset(ctx context.Context, id, portfolioID string) error {
	res := s.db.WithContext(ctx).Where("id = ? AND portfolio_id = ?", id, portfolioID).Delete(&domain.Asset{})
	if res.Error != nil {
		return translate(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
