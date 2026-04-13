package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration from environment or use defaults
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=portfolio_manager port=5432 sslmode=disable"
	}

	// Initialize database
	if err := InitDB(dsn); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Setup Gin router
	router := gin.Default()

	// Public routes
	public := router.Group("/api")
	{
		public.POST("/auth/register", Register)
		public.POST("/auth/login", Login)
	}

	// Protected routes
	protected := router.Group("/api")
	protected.Use(AuthMiddleware())
	{
		// Portfolio routes
		protected.POST("/portfolios", CreatePortfolioHandler)
		protected.GET("/portfolios", ListPortfoliosHandler)
		protected.GET("/portfolios/:id", GetPortfolioHandler)
		protected.PUT("/portfolios/:id", UpdatePortfolioHandler)
		protected.DELETE("/portfolios/:id", DeletePortfolioHandler)
		protected.GET("/portfolios/:id/stats", GetPortfolioStatsHandler)

		// Asset routes
		protected.POST("/portfolios/:id/assets", AddAssetHandler)
		protected.PUT("/portfolios/:portfolio_id/assets/:asset_id", UpdateAssetHandler)
		protected.DELETE("/portfolios/:portfolio_id/assets/:asset_id", DeleteAssetHandler)
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	log.Printf("Starting server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
