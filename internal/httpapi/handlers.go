package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/striver-21/portfolio-manager/internal/domain"
)

// ---- auth -----------------------------------------------------------------

type registerRequest struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,max=72"`
}

type loginResponse struct {
	Token  string `json:"token"`
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func (s *Server) register(c *gin.Context) {
	var req registerRequest
	if !bindJSON(c, &req) {
		return
	}
	hash, err := s.hasher.Hash(req.Password)
	if err != nil {
		s.failFromError(c, err)
		return
	}
	user := &domain.User{Email: normalizeEmail(req.Email), PasswordHash: hash}
	// No read-before-write: the unique index is the source of truth, and the
	// store reports a race-free domain.ErrConflict.
	if err := s.store.CreateUser(c.Request.Context(), user); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			fail(c, http.StatusConflict, "email already registered")
			return
		}
		s.failFromError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": user.ID, "email": user.Email})
}

func (s *Server) login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	user, err := s.store.UserByEmail(c.Request.Context(), normalizeEmail(req.Email))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.hasher.CheckDummy(req.Password) // equalise timing with the found-user path
		fail(c, http.StatusUnauthorized, "invalid credentials")
		return
	case err != nil:
		s.failFromError(c, err)
		return
	}
	if !s.hasher.Check(req.Password, user.PasswordHash) {
		fail(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		s.failFromError(c, err)
		return
	}
	c.JSON(http.StatusOK, loginResponse{Token: token, UserID: user.ID, Email: user.Email})
}

// ---- portfolios -----------------------------------------------------------

type createPortfolioRequest struct {
	Name        string `json:"name" binding:"required,max=100"`
	Description string `json:"description" binding:"max=1000"`
}

// updatePortfolioRequest uses pointers so "absent" and "empty string" are
// distinguishable: a null/missing field is left alone, "" clears it.
type updatePortfolioRequest struct {
	Name        *string `json:"name" binding:"omitempty,min=1,max=100"`
	Description *string `json:"description" binding:"omitempty,max=1000"`
}

type listResponse[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

func (s *Server) createPortfolio(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	var req createPortfolioRequest
	if !bindJSON(c, &req) {
		return
	}
	p := &domain.Portfolio{UserID: uid, Name: strings.TrimSpace(req.Name), Description: req.Description}
	if p.Name == "" {
		fail(c, http.StatusBadRequest, "validation failed", "name: must not be blank")
		return
	}
	if err := s.store.CreatePortfolio(c.Request.Context(), p); err != nil {
		s.failFromError(c, err)
		return
	}
	p.Assets = []domain.Asset{}
	c.JSON(http.StatusCreated, p)
}

func (s *Server) listPortfolios(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	page, ok := pageFrom(c)
	if !ok {
		return
	}
	items, total, err := s.store.Portfolios(c.Request.Context(), uid, page)
	if err != nil {
		s.failFromError(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponse[domain.Portfolio]{Items: items, Total: total, Limit: page.Limit, Offset: page.Offset})
}

// loadPortfolio fetches the portfolio named in the :id param for the current
// user, writing the error response itself when that fails.
func (s *Server) loadPortfolio(c *gin.Context) (*domain.Portfolio, bool) {
	uid, ok := userID(c)
	if !ok {
		return nil, false
	}
	p, err := s.store.Portfolio(c.Request.Context(), c.Param("id"), uid)
	if err != nil {
		s.failFromError(c, err)
		return nil, false
	}
	return p, true
}

func (s *Server) getPortfolio(c *gin.Context) {
	if p, ok := s.loadPortfolio(c); ok {
		c.JSON(http.StatusOK, p)
	}
}

func (s *Server) updatePortfolio(c *gin.Context) {
	p, ok := s.loadPortfolio(c)
	if !ok {
		return
	}
	var req updatePortfolioRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Name == nil && req.Description == nil {
		fail(c, http.StatusBadRequest, "validation failed", "at least one of name, description is required")
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			fail(c, http.StatusBadRequest, "validation failed", "name: must not be blank")
			return
		}
		p.Name = name
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if err := s.store.UpdatePortfolio(c.Request.Context(), p); err != nil {
		s.failFromError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (s *Server) deletePortfolio(c *gin.Context) {
	uid, ok := userID(c)
	if !ok {
		return
	}
	if err := s.store.DeletePortfolio(c.Request.Context(), c.Param("id"), uid); err != nil {
		s.failFromError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) portfolioStats(c *gin.Context) {
	p, ok := s.loadPortfolio(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, domain.CalculateStats(p.Assets))
}

// ---- assets ---------------------------------------------------------------

type addAssetRequest struct {
	Symbol        string          `json:"symbol" binding:"required,max=32"`
	Quantity      decimal.Decimal `json:"quantity"`
	PurchasePrice decimal.Decimal `json:"purchase_price"`
	CurrentPrice  decimal.Decimal `json:"current_price"`
}

type updateAssetRequest struct {
	Quantity      *decimal.Decimal `json:"quantity"`
	PurchasePrice *decimal.Decimal `json:"purchase_price"`
	CurrentPrice  *decimal.Decimal `json:"current_price"`
}

func (s *Server) addAsset(c *gin.Context) {
	p, ok := s.loadPortfolio(c)
	if !ok {
		return
	}
	var req addAssetRequest
	if !bindJSON(c, &req) {
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		fail(c, http.StatusBadRequest, "validation failed", "symbol: must not be blank")
		return
	}
	var details []string
	for _, f := range []struct {
		name string
		val  decimal.Decimal
	}{{"quantity", req.Quantity}, {"purchase_price", req.PurchasePrice}, {"current_price", req.CurrentPrice}} {
		if msg, ok := validAmount(f.name, f.val); !ok {
			details = append(details, msg)
		}
	}
	if len(details) > 0 {
		fail(c, http.StatusBadRequest, "validation failed", details...)
		return
	}
	a := &domain.Asset{
		PortfolioID:   p.ID,
		Symbol:        symbol,
		Quantity:      req.Quantity,
		PurchasePrice: req.PurchasePrice,
		CurrentPrice:  req.CurrentPrice,
	}
	if err := s.store.CreateAsset(c.Request.Context(), a); err != nil {
		s.failFromError(c, err)
		return
	}
	c.JSON(http.StatusCreated, a)
}

func (s *Server) updateAsset(c *gin.Context) {
	p, ok := s.loadPortfolio(c)
	if !ok {
		return
	}
	a, err := s.store.Asset(c.Request.Context(), c.Param("asset_id"), p.ID)
	if err != nil {
		s.failFromError(c, err)
		return
	}
	var req updateAssetRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Quantity == nil && req.PurchasePrice == nil && req.CurrentPrice == nil {
		fail(c, http.StatusBadRequest, "validation failed", "at least one of quantity, purchase_price, current_price is required")
		return
	}
	var details []string
	apply := func(name string, src *decimal.Decimal, dst *decimal.Decimal) {
		if src == nil {
			return
		}
		if msg, ok := validAmount(name, *src); !ok {
			details = append(details, msg)
			return
		}
		*dst = *src
	}
	apply("quantity", req.Quantity, &a.Quantity)
	apply("purchase_price", req.PurchasePrice, &a.PurchasePrice)
	apply("current_price", req.CurrentPrice, &a.CurrentPrice)
	if len(details) > 0 {
		fail(c, http.StatusBadRequest, "validation failed", details...)
		return
	}
	if err := s.store.UpdateAsset(c.Request.Context(), a); err != nil {
		s.failFromError(c, err)
		return
	}
	c.JSON(http.StatusOK, a)
}

func (s *Server) deleteAsset(c *gin.Context) {
	p, ok := s.loadPortfolio(c)
	if !ok {
		return
	}
	if err := s.store.DeleteAsset(c.Request.Context(), c.Param("asset_id"), p.ID); err != nil {
		s.failFromError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
