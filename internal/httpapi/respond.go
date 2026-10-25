package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/shopspring/decimal"

	"github.com/striver-21/portfolio-manager/internal/domain"
	"github.com/striver-21/portfolio-manager/internal/store"
)

// errorResponse is the body of every non-2xx response.
type errorResponse struct {
	Error   string   `json:"error"`
	Details []string `json:"details,omitempty"`
}

// fail writes a JSON error and aborts the chain.
func fail(c *gin.Context, status int, msg string, details ...string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: msg, Details: details})
}

// failFromError maps a store/domain error onto a status code. Unknown errors
// become a generic 500 and are attached to the context for the request log.
func (s *Server) failFromError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		fail(c, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrConflict):
		fail(c, http.StatusConflict, "already exists")
	default:
		_ = c.Error(err)
		fail(c, http.StatusInternalServerError, "internal error")
	}
}

// bindJSON decodes and validates the body. On failure it writes a 400 with
// field-level details and returns false.
func bindJSON(c *gin.Context, dst any) bool {
	err := c.ShouldBindJSON(dst)
	if err == nil {
		return true
	}
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		details := make([]string, 0, len(verrs))
		for _, fe := range verrs {
			details = append(details, fmt.Sprintf("%s: failed %q", strings.ToLower(fe.Field()), fe.Tag()))
		}
		fail(c, http.StatusBadRequest, "validation failed", details...)
		return false
	}
	fail(c, http.StatusBadRequest, "malformed JSON body")
	return false
}

// pageFrom parses ?limit= and ?offset=. Out-of-range values are clamped by
// store.Page.Normalize; non-numeric values are a 400.
func pageFrom(c *gin.Context) (store.Page, bool) {
	var page store.Page
	for name, dst := range map[string]*int{"limit": &page.Limit, "offset": &page.Offset} {
		raw := c.Query(name)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			fail(c, http.StatusBadRequest, "invalid query parameter", name+": must be a non-negative integer")
			return store.Page{}, false
		}
		*dst = n
	}
	return page.Normalize(), true
}

// Bounds matching the NUMERIC(24,8) columns in the Postgres schema.
const (
	maxAmountScale  = 8
	maxAmountDigits = 16 // integer digits
)

var maxAmount = decimal.New(1, maxAmountDigits) // 10^16

// validAmount checks a quantity or price fits the storage column and is
// strictly positive. It returns a message suitable for the client.
func validAmount(name string, d decimal.Decimal) (string, bool) {
	switch {
	case !d.IsPositive():
		return name + ": must be greater than 0", false
	case !d.Round(maxAmountScale).Equal(d):
		return fmt.Sprintf("%s: at most %d decimal places", name, maxAmountScale), false
	case d.GreaterThanOrEqual(maxAmount):
		return fmt.Sprintf("%s: must be below %s", name, maxAmount.String()), false
	}
	return "", true
}
