package main

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a portfolio owner
type User struct {
	ID         string      `gorm:"primaryKey" json:"id"`
	Email      string      `gorm:"uniqueIndex" json:"email"`
	Password   string      `json:"-"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	Portfolios []Portfolio `gorm:"foreignKey:UserID" json:"portfolios,omitempty"`
}

// Portfolio represents a user's investment portfolio
type Portfolio struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	UserID      string    `gorm:"index" json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Assets      []Asset   `gorm:"foreignKey:PortfolioID;constraint:OnDelete:CASCADE" json:"assets,omitempty"`
	User        User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// Asset represents a holding in a portfolio
type Asset struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	PortfolioID   string    `gorm:"index" json:"portfolio_id"`
	Symbol        string    `json:"symbol"`
	Quantity      float64   `json:"quantity"`
	PurchasePrice float64   `json:"purchase_price"`
	CurrentPrice  float64   `json:"current_price"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Portfolio     Portfolio `gorm:"foreignKey:PortfolioID" json:"portfolio,omitempty"`
}

// PortfolioStats contains calculated portfolio statistics
type PortfolioStats struct {
	TotalValue       float64            `json:"total_value"`
	TotalCost        float64            `json:"total_cost"`
	TotalGain        float64            `json:"total_gain"`
	TotalGainPercent float64            `json:"total_gain_percent"`
	AssetCount       int                `json:"asset_count"`
	AssetBreakdown   map[string]float64 `json:"asset_breakdown"`
}

// CreateUserRequest is the request body for creating a user
type CreateUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// CreatePortfolioRequest is the request body for creating a portfolio
type CreatePortfolioRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UpdatePortfolioRequest is the request body for updating a portfolio
type UpdatePortfolioRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AddAssetRequest is the request body for adding an asset to a portfolio
type AddAssetRequest struct {
	Symbol        string  `json:"symbol" binding:"required"`
	Quantity      float64 `json:"quantity" binding:"required,gt=0"`
	PurchasePrice float64 `json:"purchase_price" binding:"required,gt=0"`
	CurrentPrice  float64 `json:"current_price" binding:"required,gt=0"`
}

// UpdateAssetRequest is the request body for updating an asset
type UpdateAssetRequest struct {
	Quantity      *float64 `json:"quantity"`
	PurchasePrice *float64 `json:"purchase_price"`
	CurrentPrice  *float64 `json:"current_price"`
}

// BeforeCreate hook to auto-generate ID
func (u *User) BeforeCreate(tx *gorm.DB) error {
	u.ID = uuid.New().String()
	return nil
}

// BeforeCreate hook to auto-generate ID
func (p *Portfolio) BeforeCreate(tx *gorm.DB) error {
	p.ID = uuid.New().String()
	return nil
}

// BeforeCreate hook to auto-generate ID
func (a *Asset) BeforeCreate(tx *gorm.DB) error {
	a.ID = uuid.New().String()
	return nil
}
