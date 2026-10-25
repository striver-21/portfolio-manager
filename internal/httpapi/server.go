// Package httpapi wires the REST endpoints: routing, middleware, request
// validation and the mapping from domain errors to HTTP status codes.
package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/striver-21/portfolio-manager/internal/auth"
	"github.com/striver-21/portfolio-manager/internal/ratelimit"
	"github.com/striver-21/portfolio-manager/internal/store"
)

// Options are the dependencies of a Server.
type Options struct {
	Store  store.Store
	Tokens *auth.TokenManager
	// Hasher hashes passwords. nil uses bcrypt's default cost.
	Hasher *auth.PasswordHasher
	// Logger receives one record per request plus any 5xx cause. nil discards.
	Logger *slog.Logger
	// AuthLimiter throttles /api/auth/* per client IP. nil disables throttling.
	AuthLimiter *ratelimit.Limiter
}

// Server is the HTTP API. Construct it with New and serve Handler().
type Server struct {
	store   store.Store
	tokens  *auth.TokenManager
	hasher  *auth.PasswordHasher
	log     *slog.Logger
	limiter *ratelimit.Limiter
	engine  *gin.Engine
}

// New builds the router. It panics only on programmer error (nil Store or
// Tokens), which is the right behaviour for a constructor called from main.
func New(opts Options) *Server {
	if opts.Store == nil || opts.Tokens == nil {
		panic("httpapi.New: Store and Tokens are required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opts.Hasher == nil {
		opts.Hasher = &auth.PasswordHasher{}
	}
	s := &Server{
		store:   opts.Store,
		tokens:  opts.Tokens,
		hasher:  opts.Hasher,
		log:     opts.Logger,
		limiter: opts.AuthLimiter,
	}
	s.engine = s.routes()
	return s
}

// Handler returns the http.Handler to mount on an http.Server.
func (s *Server) Handler() http.Handler { return s.engine }

func (s *Server) routes() *gin.Engine {
	r := gin.New()
	// Trust no proxy headers by default: ClientIP() is then the TCP peer,
	// which is what the rate limiter must key on. Deploy behind a proxy by
	// configuring trusted CIDRs here.
	_ = r.SetTrustedProxies(nil)
	r.HandleMethodNotAllowed = true
	r.Use(requestID(), s.logging(), s.recovery())

	r.NoRoute(func(c *gin.Context) { fail(c, http.StatusNotFound, "route not found") })
	r.NoMethod(func(c *gin.Context) { fail(c, http.StatusMethodNotAllowed, "method not allowed") })

	r.GET("/health", s.health)

	api := r.Group("/api")

	authGroup := api.Group("/auth")
	if s.limiter != nil {
		authGroup.Use(rateLimit(s.limiter))
	}
	authGroup.POST("/register", s.register)
	authGroup.POST("/login", s.login)

	protected := api.Group("", s.requireAuth())
	protected.POST("/portfolios", s.createPortfolio)
	protected.GET("/portfolios", s.listPortfolios)
	protected.GET("/portfolios/:id", s.getPortfolio)
	protected.PUT("/portfolios/:id", s.updatePortfolio)
	protected.DELETE("/portfolios/:id", s.deletePortfolio)
	protected.GET("/portfolios/:id/stats", s.portfolioStats)
	protected.POST("/portfolios/:id/assets", s.addAsset)
	protected.PUT("/portfolios/:id/assets/:asset_id", s.updateAsset)
	protected.DELETE("/portfolios/:id/assets/:asset_id", s.deleteAsset)

	return r
}

func (s *Server) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		s.log.ErrorContext(ctx, "health check failed", "error", err, "request_id", requestIDFrom(c))
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
