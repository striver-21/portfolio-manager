package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/striver-21/portfolio-manager/internal/ratelimit"
)

// Context keys for values stored on *gin.Context. Unexported strings keep the
// keys private to this package.
const (
	ctxRequestID = "httpapi.request_id"
	ctxUserID    = "httpapi.user_id"
)

// requestIDHeader is honoured on the way in (when well-formed) and always set
// on the way out so clients and logs can be correlated.
const requestIDHeader = "X-Request-ID"

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if !validRequestID(id) {
			id = uuid.NewString()
		}
		c.Set(ctxRequestID, id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

// validRequestID accepts short printable ASCII so a client cannot inject
// control characters or megabytes into the logs.
func validRequestID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

func requestIDFrom(c *gin.Context) string { return c.GetString(ctxRequestID) }

func (s *Server) logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", c.Writer.Size(),
			"ip", c.ClientIP(),
			"request_id", requestIDFrom(c),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "errors", c.Errors.String())
		}
		switch status := c.Writer.Status(); {
		case status >= http.StatusInternalServerError:
			s.log.ErrorContext(c.Request.Context(), "request", attrs...)
		default:
			s.log.InfoContext(c.Request.Context(), "request", attrs...)
		}
	}
}

func (s *Server) recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		s.log.ErrorContext(c.Request.Context(), "panic recovered",
			"panic", fmt.Sprint(recovered), "request_id", requestIDFrom(c))
		fail(c, http.StatusInternalServerError, "internal error")
	})
}

// requireAuth validates the bearer token and stores the user ID on the context.
func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			c.Header("WWW-Authenticate", `Bearer realm="portfolio-manager"`)
			fail(c, http.StatusUnauthorized, "missing or malformed authorization header")
			return
		}
		claims, err := s.tokens.Verify(token)
		if err != nil {
			c.Header("WWW-Authenticate", `Bearer realm="portfolio-manager", error="invalid_token"`)
			fail(c, http.StatusUnauthorized, "invalid token")
			return
		}
		c.Set(ctxUserID, claims.UserID)
		c.Next()
	}
}

func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// errNoUser is returned when a handler runs without requireAuth in front of it.
var errNoUser = errors.New("no authenticated user on request")

// userID returns the authenticated user or writes a 401 and returns false.
func userID(c *gin.Context) (string, bool) {
	id := c.GetString(ctxUserID)
	if id == "" {
		_ = c.Error(errNoUser)
		fail(c, http.StatusUnauthorized, "authentication required")
		return "", false
	}
	return id, true
}

func rateLimit(l *ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(c.ClientIP()) {
			c.Header("Retry-After", "60")
			fail(c, http.StatusTooManyRequests, "too many requests")
			return
		}
		c.Next()
	}
}
