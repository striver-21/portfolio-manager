package main

import (
	"errors"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB is the global database connection
var DB *gorm.DB

// InitDB initializes the database connection
func InitDB(dsn string) error {
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	// AutoMigrate creates tables if they don't exist
	if err := DB.AutoMigrate(&User{}, &Portfolio{}, &Asset{}); err != nil {
		return err
	}

	log.Println("Database initialized successfully")
	return nil
}

// CreateUser creates a new user in the database
func CreateUser(user *User) error {
	if err := DB.Create(user).Error; err != nil {
		return err
	}
	return nil
}

// GetUserByEmail retrieves a user by email
func GetUserByEmail(email string) (*User, error) {
	var user User
	if err := DB.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByID retrieves a user by ID
func GetUserByID(id string) (*User, error) {
	var user User
	if err := DB.First(&user, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// CreatePortfolio creates a new portfolio for a user
func CreatePortfolio(portfolio *Portfolio) error {
	return DB.Create(portfolio).Error
}

// GetPortfolio retrieves a portfolio by ID and validates ownership
func GetPortfolio(portfolioID, userID string) (*Portfolio, error) {
	var portfolio Portfolio
	if err := DB.Where("id = ? AND user_id = ?", portfolioID, userID).
		Preload("Assets").
		First(&portfolio).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &portfolio, nil
}

// GetUserPortfolios retrieves all portfolios for a user
func GetUserPortfolios(userID string) ([]Portfolio, error) {
	var portfolios []Portfolio
	if err := DB.Where("user_id = ?", userID).
		Preload("Assets").
		Find(&portfolios).Error; err != nil {
		return nil, err
	}
	return portfolios, nil
}

// UpdatePortfolio updates a portfolio
func UpdatePortfolio(portfolio *Portfolio) error {
	return DB.Model(portfolio).Updates(portfolio).Error
}

// DeletePortfolio deletes a portfolio and its assets
func DeletePortfolio(portfolioID, userID string) error {
	return DB.Where("id = ? AND user_id = ?", portfolioID, userID).Delete(&Portfolio{}).Error
}

// AddAsset adds a new asset to a portfolio
func AddAsset(asset *Asset) error {
	return DB.Create(asset).Error
}

// GetAsset retrieves an asset with ownership validation
func GetAsset(assetID, portfolioID string) (*Asset, error) {
	var asset Asset
	if err := DB.Where("id = ? AND portfolio_id = ?", assetID, portfolioID).
		First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &asset, nil
}

// UpdateAsset updates an asset
func UpdateAsset(asset *Asset) error {
	return DB.Model(asset).Updates(asset).Error
}

// DeleteAsset deletes an asset
func DeleteAsset(assetID, portfolioID string) error {
	return DB.Where("id = ? AND portfolio_id = ?", assetID, portfolioID).Delete(&Asset{}).Error
}

// GetPortfolioAssets retrieves all assets in a portfolio
func GetPortfolioAssets(portfolioID string) ([]Asset, error) {
	var assets []Asset
	if err := DB.Where("portfolio_id = ?", portfolioID).Find(&assets).Error; err != nil {
		return nil, err
	}
	return assets, nil
}
