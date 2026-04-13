package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB() {
	var err error
	DB, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic("failed to initialize test database")
	}

	// Run migrations
	DB.AutoMigrate(&User{}, &Portfolio{}, &Asset{})
}

// TestRegisterSuccess tests successful user registration
func TestRegisterSuccess(t *testing.T) {
	setupTestDB()

	router := setupTestRouter()

	body := CreateUserRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.NotEmpty(t, response["id"])
	assert.Equal(t, "test@example.com", response["email"])
}

// TestRegisterDuplicateEmail tests that duplicate email registration fails
func TestRegisterDuplicateEmail(t *testing.T) {
	setupTestDB()

	router := setupTestRouter()

	body := CreateUserRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	bodyBytes, _ := json.Marshal(body)

	// First registration
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Second registration with same email
	req, _ = http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
}

// TestLoginSuccess tests successful login
func TestLoginSuccess(t *testing.T) {
	setupTestDB()

	router := setupTestRouter()

	// Register user first
	registerBody := CreateUserRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	registerBytes, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(registerBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Login
	loginBody := LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	loginBytes, _ := json.Marshal(loginBody)
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(loginBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response LoginResponse
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.NotEmpty(t, response.Token)
	assert.Equal(t, "test@example.com", response.Email)
}

// TestLoginInvalidPassword tests that login fails with invalid password
func TestLoginInvalidPassword(t *testing.T) {
	setupTestDB()

	router := setupTestRouter()

	// Register user
	registerBody := CreateUserRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	registerBytes, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(registerBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Login with wrong password
	loginBody := LoginRequest{
		Email:    "test@example.com",
		Password: "wrongpassword",
	}
	loginBytes, _ := json.Marshal(loginBody)
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(loginBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestCreatePortfolio tests portfolio creation
func TestCreatePortfolio(t *testing.T) {
	setupTestDB()

	router := setupTestRouter()

	// Register and login
	token := registerAndLogin(router)

	body := CreatePortfolioRequest{
		Name:        "My Portfolio",
		Description: "My first portfolio",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/portfolios", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response Portfolio
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "My Portfolio", response.Name)
	assert.Equal(t, "My first portfolio", response.Description)
}

// TestListPortfolios tests retrieving user portfolios
func TestListPortfolios(t *testing.T) {
	setupTestDB()

	router := setupTestRouter()

	// Register and login
	token := registerAndLogin(router)

	// Create two portfolios
	for i := 0; i < 2; i++ {
		body := CreatePortfolioRequest{
			Name: "Portfolio " + string(rune(i+1)),
		}
		bodyBytes, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/api/portfolios", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// List portfolios
	req, _ := http.NewRequest("GET", "/api/portfolios", nil)
	req.Header.Set("Authorization", "Bearer " + token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response []Portfolio
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, 2, len(response))
}

// TestCalculatePortfolioStats tests portfolio statistics calculation
func TestCalculatePortfolioStats(t *testing.T) {
	portfolio := &Portfolio{
		Assets: []Asset{
			{
				Symbol:        "AAPL",
				Quantity:      10,
				PurchasePrice: 100,
				CurrentPrice:  150,
			},
			{
				Symbol:        "GOOGL",
				Quantity:      5,
				PurchasePrice: 200,
				CurrentPrice:  250,
			},
		},
	}

	stats, err := CalculatePortfolioStats(portfolio)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, 2, stats.AssetCount)
	assert.Equal(t, float64(4250), stats.TotalValue)    // (10*150) + (5*250)
	assert.Equal(t, float64(3500), stats.TotalCost)     // (10*100) + (5*200)
	assert.Equal(t, float64(750), stats.TotalGain)      // 4250 - 3500
	assert.InDelta(t, 21.43, stats.TotalGainPercent, 1) // (750/3500)*100
}

// setupTestRouter creates a router for testing
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.Default()

	public := router.Group("/api")
	{
		public.POST("/auth/register", Register)
		public.POST("/auth/login", Login)
	}

	protected := router.Group("/api")
	protected.Use(AuthMiddleware())
	{
		protected.POST("/portfolios", CreatePortfolioHandler)
		protected.GET("/portfolios", ListPortfoliosHandler)
		protected.GET("/portfolios/:id", GetPortfolioHandler)
		protected.PUT("/portfolios/:id", UpdatePortfolioHandler)
		protected.DELETE("/portfolios/:id", DeletePortfolioHandler)
		protected.GET("/portfolios/:id/stats", GetPortfolioStatsHandler)

		protected.POST("/portfolios/:id/assets", AddAssetHandler)
		protected.PUT("/portfolios/:portfolio_id/assets/:asset_id", UpdateAssetHandler)
		protected.DELETE("/portfolios/:portfolio_id/assets/:asset_id", DeleteAssetHandler)
	}

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	return router
}

// registerAndLogin is a helper function to register and login a user
func registerAndLogin(router *gin.Engine) string {
	registerBody := CreateUserRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	registerBytes, _ := json.Marshal(registerBody)
	req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(registerBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	loginBody := LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	loginBytes, _ := json.Marshal(loginBody)
	req, _ = http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(loginBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response LoginResponse
	json.Unmarshal(w.Body.Bytes(), &response)
	return response.Token
}
