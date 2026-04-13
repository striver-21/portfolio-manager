package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Register creates a new user account
func Register(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if user already exists
	existingUser, err := GetUserByEmail(req.Email)
	if err != nil {
		log.Printf("Database error checking user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if existingUser != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
		return
	}

	// Hash password
	hashedPassword, err := HashPassword(req.Password)
	if err != nil {
		log.Printf("Password hashing error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hashing failed"})
		return
	}

	// Create user
	user := &User{
		Email:    req.Email,
		Password: hashedPassword,
	}

	if err := CreateUser(user); err != nil {
		log.Printf("Database error creating user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":    user.ID,
		"email": user.Email,
	})
}

// Login authenticates a user and returns a JWT token
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user by email
	user, err := GetUserByEmail(req.Email)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Verify password
	if !CheckPassword(req.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Generate token
	token, err := GenerateToken(user.ID)
	if err != nil {
		log.Printf("Token generation error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token:  token,
		UserID: user.ID,
		Email:  user.Email,
	})
}

// CreatePortfolioHandler creates a new portfolio for the authenticated user
func CreatePortfolioHandler(c *gin.Context) {
	userID := GetUserID(c)

	var req CreatePortfolioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	portfolio := &Portfolio{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := CreatePortfolio(portfolio); err != nil {
		log.Printf("Database error creating portfolio: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create portfolio"})
		return
	}

	c.JSON(http.StatusCreated, portfolio)
}

// GetPortfolioHandler retrieves a specific portfolio
func GetPortfolioHandler(c *gin.Context) {
	userID := GetUserID(c)
	portfolioID := c.Param("id")

	portfolio, err := GetPortfolio(portfolioID, userID)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if portfolio == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "portfolio not found"})
		return
	}

	c.JSON(http.StatusOK, portfolio)
}

// ListPortfoliosHandler retrieves all portfolios for the user
func ListPortfoliosHandler(c *gin.Context) {
	userID := GetUserID(c)

	portfolios, err := GetUserPortfolios(userID)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if portfolios == nil {
		portfolios = []Portfolio{}
	}
	c.JSON(http.StatusOK, portfolios)
}

// UpdatePortfolioHandler updates a portfolio
func UpdatePortfolioHandler(c *gin.Context) {
	userID := GetUserID(c)
	portfolioID := c.Param("id")

	portfolio, err := GetPortfolio(portfolioID, userID)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if portfolio == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "portfolio not found"})
		return
	}

	var req UpdatePortfolioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		portfolio.Name = req.Name
	}
	if req.Description != "" {
		portfolio.Description = req.Description
	}

	if err := UpdatePortfolio(portfolio); err != nil {
		log.Printf("Database error updating portfolio: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update portfolio"})
		return
	}

	c.JSON(http.StatusOK, portfolio)
}

// DeletePortfolioHandler deletes a portfolio
func DeletePortfolioHandler(c *gin.Context) {
	userID := GetUserID(c)
	portfolioID := c.Param("id")

	if err := DeletePortfolio(portfolioID, userID); err != nil {
		log.Printf("Database error deleting portfolio: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete portfolio"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "portfolio deleted"})
}

// AddAssetHandler adds a new asset to a portfolio
func AddAssetHandler(c *gin.Context) {
	userID := GetUserID(c)
	portfolioID := c.Param("id")

	// Verify portfolio ownership
	portfolio, err := GetPortfolio(portfolioID, userID)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if portfolio == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "portfolio not found"})
		return
	}

	var req AddAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	asset := &Asset{
		PortfolioID:   portfolioID,
		Symbol:        req.Symbol,
		Quantity:      req.Quantity,
		PurchasePrice: req.PurchasePrice,
		CurrentPrice:  req.CurrentPrice,
	}

	if err := AddAsset(asset); err != nil {
		log.Printf("Database error adding asset: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add asset"})
		return
	}

	c.JSON(http.StatusCreated, asset)
}

// UpdateAssetHandler updates an asset in a portfolio
func UpdateAssetHandler(c *gin.Context) {
	userID := GetUserID(c)
	portfolioID := c.Param("portfolio_id")
	assetID := c.Param("asset_id")

	// Verify portfolio ownership
	portfolio, err := GetPortfolio(portfolioID, userID)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if portfolio == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "portfolio not found"})
		return
	}

	// Get asset
	asset, err := GetAsset(assetID, portfolioID)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if asset == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found"})
		return
	}

	var req UpdateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Quantity != nil {
		asset.Quantity = *req.Quantity
	}
	if req.PurchasePrice != nil {
		asset.PurchasePrice = *req.PurchasePrice
	}
	if req.CurrentPrice != nil {
		asset.CurrentPrice = *req.CurrentPrice
	}

	if err := UpdateAsset(asset); err != nil {
		log.Printf("Database error updating asset: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update asset"})
		return
	}

	c.JSON(http.StatusOK, asset)
}

// DeleteAssetHandler deletes an asset from a portfolio
func DeleteAssetHandler(c *gin.Context) {
	userID := GetUserID(c)
	portfolioID := c.Param("portfolio_id")
	assetID := c.Param("asset_id")

	// Verify portfolio ownership
	portfolio, err := GetPortfolio(portfolioID, userID)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if portfolio == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "portfolio not found"})
		return
	}

	if err := DeleteAsset(assetID, portfolioID); err != nil {
		log.Printf("Database error deleting asset: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete asset"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "asset deleted"})
}

// GetPortfolioStatsHandler calculates and returns portfolio statistics
func GetPortfolioStatsHandler(c *gin.Context) {
	userID := GetUserID(c)
	portfolioID := c.Param("id")

	portfolio, err := GetPortfolio(portfolioID, userID)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if portfolio == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "portfolio not found"})
		return
	}

	stats, err := CalculatePortfolioStats(portfolio)
	if err != nil {
		log.Printf("Stats calculation error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to calculate stats"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// CalculatePortfolioStats calculates portfolio statistics
func CalculatePortfolioStats(portfolio *Portfolio) (*PortfolioStats, error) {
	if portfolio == nil {
		return nil, errors.New("portfolio is nil")
	}

	stats := &PortfolioStats{
		AssetBreakdown: make(map[string]float64),
	}

	for _, asset := range portfolio.Assets {
		cost := asset.PurchasePrice * asset.Quantity
		value := asset.CurrentPrice * asset.Quantity
		gain := value - cost

		stats.TotalCost += cost
		stats.TotalValue += value
		stats.TotalGain += gain
		stats.AssetCount++
		stats.AssetBreakdown[asset.Symbol] = value
	}

	if stats.TotalCost > 0 {
		stats.TotalGainPercent = (stats.TotalGain / stats.TotalCost) * 100
	}

	return stats, nil
}
