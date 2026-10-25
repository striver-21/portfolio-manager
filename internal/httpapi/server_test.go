package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/striver-21/portfolio-manager/internal/auth"
	"github.com/striver-21/portfolio-manager/internal/domain"
	"github.com/striver-21/portfolio-manager/internal/httpapi"
	"github.com/striver-21/portfolio-manager/internal/ratelimit"
	"github.com/striver-21/portfolio-manager/internal/store"
	"github.com/striver-21/portfolio-manager/internal/store/storetest"
)

func init() { gin.SetMode(gin.TestMode) }

var testSecret = []byte(strings.Repeat("t", 32))

// testServer bundles a fresh handler with its own private database.
type testServer struct {
	t       *testing.T
	handler http.Handler
	tokens  *auth.TokenManager
}

func newTestServer(t *testing.T, limiter *ratelimit.Limiter) *testServer {
	t.Helper()
	tokens, err := auth.NewTokenManager(testSecret, "test", time.Hour)
	require.NoError(t, err)
	srv := httpapi.New(httpapi.Options{
		Store:       store.NewGorm(storetest.Open(t)),
		Tokens:      tokens,
		Hasher:      &auth.PasswordHasher{Cost: bcrypt.MinCost},
		AuthLimiter: limiter,
	})
	return &testServer{t: t, handler: srv.Handler(), tokens: tokens}
}

// do performs one request. body may be nil, a string (sent verbatim) or any
// value to be JSON-encoded.
func (ts *testServer) do(method, path, token string, body any) *httptest.ResponseRecorder {
	ts.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		require.NoError(ts.t, err)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequestWithContext(ts.t.Context(), method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	ts.handler.ServeHTTP(w, req)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v), "body: %s", w.Body.String())
	return v
}

func (ts *testServer) register(email, password string) *httptest.ResponseRecorder {
	ts.t.Helper()
	return ts.do(http.MethodPost, "/api/auth/register", "", gin.H{"email": email, "password": password})
}

// login registers (ignoring conflicts) and logs in, returning a bearer token.
func (ts *testServer) login(email string) string {
	ts.t.Helper()
	ts.register(email, "password-123")
	w := ts.do(http.MethodPost, "/api/auth/login", "", gin.H{"email": email, "password": "password-123"})
	require.Equal(ts.t, http.StatusOK, w.Code, w.Body.String())
	return decode[map[string]string](ts.t, w)["token"]
}

func (ts *testServer) createPortfolio(token, name string) domain.Portfolio {
	ts.t.Helper()
	w := ts.do(http.MethodPost, "/api/portfolios", token, gin.H{"name": name, "description": "d"})
	require.Equal(ts.t, http.StatusCreated, w.Code, w.Body.String())
	return decode[domain.Portfolio](ts.t, w)
}

func (ts *testServer) addAsset(token, portfolioID, symbol, qty, buy, now string) domain.Asset {
	ts.t.Helper()
	w := ts.do(http.MethodPost, "/api/portfolios/"+portfolioID+"/assets", token,
		gin.H{"symbol": symbol, "quantity": qty, "purchase_price": buy, "current_price": now})
	require.Equal(ts.t, http.StatusCreated, w.Code, w.Body.String())
	return decode[domain.Asset](ts.t, w)
}

type errBody struct {
	Error   string   `json:"error"`
	Details []string `json:"details"`
}

// ---- auth -----------------------------------------------------------------

func TestRegister(t *testing.T) {
	ts := newTestServer(t, nil)

	w := ts.register("New.User@Example.COM", "password-123")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	body := decode[map[string]string](t, w)
	assert.NotEmpty(t, body["id"])
	assert.Equal(t, "new.user@example.com", body["email"], "email is normalised")

	w = ts.register("new.user@example.com", "another-pass")
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "email already registered", decode[errBody](t, w).Error)

	w = ts.register("not-an-email", "short")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	e := decode[errBody](t, w)
	assert.Equal(t, "validation failed", e.Error)
	assert.Len(t, e.Details, 2)

	w = ts.do(http.MethodPost, "/api/auth/register", "", "{not json")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "malformed JSON body", decode[errBody](t, w).Error)
}

func TestLogin(t *testing.T) {
	ts := newTestServer(t, nil)
	require.Equal(t, http.StatusCreated, ts.register("a@example.com", "password-123").Code)

	w := ts.do(http.MethodPost, "/api/auth/login", "", gin.H{"email": "A@example.com", "password": "password-123"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode[map[string]string](t, w)
	assert.NotEmpty(t, body["token"])
	assert.NotEmpty(t, body["user_id"])
	assert.Equal(t, "a@example.com", body["email"])

	claims, err := ts.tokens.Verify(body["token"])
	require.NoError(t, err)
	assert.Equal(t, body["user_id"], claims.UserID)

	w = ts.do(http.MethodPost, "/api/auth/login", "", gin.H{"email": "a@example.com", "password": "wrong-password"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w = ts.do(http.MethodPost, "/api/auth/login", "", gin.H{"email": "nobody@example.com", "password": "password-123"})
	assert.Equal(t, http.StatusUnauthorized, w.Code, "unknown user is indistinguishable from wrong password")
	assert.Equal(t, "invalid credentials", decode[errBody](t, w).Error)
}

func TestAuthRateLimit(t *testing.T) {
	ts := newTestServer(t, ratelimit.New(0.001, 3))
	for i := 0; i < 3; i++ {
		w := ts.do(http.MethodPost, "/api/auth/login", "", gin.H{"email": "x@example.com", "password": "password-123"})
		assert.Equal(t, http.StatusUnauthorized, w.Code, "attempt %d", i+1)
	}
	w := ts.do(http.MethodPost, "/api/auth/login", "", gin.H{"email": "x@example.com", "password": "password-123"})
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "60", w.Header().Get("Retry-After"))

	// Register shares the bucket, the protected API does not.
	assert.Equal(t, http.StatusTooManyRequests, ts.register("y@example.com", "password-123").Code)
	assert.Equal(t, http.StatusUnauthorized, ts.do(http.MethodGet, "/api/portfolios", "", nil).Code)
}

func TestProtectedRoutesRequireValidToken(t *testing.T) {
	ts := newTestServer(t, nil)

	w := ts.do(http.MethodGet, "/api/portfolios", "", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/portfolios", nil)
	req.Header.Set("Authorization", "Basic abc")
	w = httptest.NewRecorder()
	ts.handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	w = ts.do(http.MethodGet, "/api/portfolios", "not.a.jwt", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "invalid token", decode[errBody](t, w).Error)

	other, err := auth.NewTokenManager([]byte(strings.Repeat("o", 32)), "test", time.Hour)
	require.NoError(t, err)
	forged, err := other.Issue("someone")
	require.NoError(t, err)
	w = ts.do(http.MethodGet, "/api/portfolios", forged, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---- portfolios -----------------------------------------------------------

func TestPortfolioLifecycle(t *testing.T) {
	ts := newTestServer(t, nil)
	token := ts.login("owner@example.com")

	// Create
	w := ts.do(http.MethodPost, "/api/portfolios", token, gin.H{"name": "  Retirement ", "description": "long term"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decode[domain.Portfolio](t, w)
	assert.Equal(t, "Retirement", created.Name)
	assert.Equal(t, "long term", created.Description)
	assert.NotEmpty(t, created.ID)
	assert.Contains(t, w.Body.String(), `"assets":[]`)
	assert.NotContains(t, w.Body.String(), `"user":`, "no embedded zero-valued user object")

	w = ts.do(http.MethodPost, "/api/portfolios", token, gin.H{"name": "   "})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Get
	w = ts.do(http.MethodGet, "/api/portfolios/"+created.ID, token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, created.ID, decode[domain.Portfolio](t, w).ID)

	// Update: clearing the description must stick.
	w = ts.do(http.MethodPut, "/api/portfolios/"+created.ID, token, gin.H{"description": ""})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := decode[domain.Portfolio](t, w)
	assert.Equal(t, "Retirement", updated.Name, "name untouched when absent")
	assert.Equal(t, "", updated.Description)

	w = ts.do(http.MethodGet, "/api/portfolios/"+created.ID, token, nil)
	assert.Equal(t, "", decode[domain.Portfolio](t, w).Description, "cleared description persisted")

	w = ts.do(http.MethodPut, "/api/portfolios/"+created.ID, token, gin.H{"name": "Growth"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Growth", decode[domain.Portfolio](t, w).Name)

	w = ts.do(http.MethodPut, "/api/portfolios/"+created.ID, token, gin.H{})
	assert.Equal(t, http.StatusBadRequest, w.Code, "empty update is rejected")
	w = ts.do(http.MethodPut, "/api/portfolios/"+created.ID, token, gin.H{"name": ""})
	assert.Equal(t, http.StatusBadRequest, w.Code, "blank name is rejected")

	// Delete: 204 once, 404 after.
	w = ts.do(http.MethodDelete, "/api/portfolios/"+created.ID, token, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
	w = ts.do(http.MethodDelete, "/api/portfolios/"+created.ID, token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = ts.do(http.MethodGet, "/api/portfolios/"+created.ID, token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPortfolioIsolationBetweenUsers(t *testing.T) {
	ts := newTestServer(t, nil)
	alice := ts.login("alice@example.com")
	bob := ts.login("bob@example.com")
	p := ts.createPortfolio(alice, "Alice's")

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/portfolios/" + p.ID},
		{http.MethodPut, "/api/portfolios/" + p.ID},
		{http.MethodDelete, "/api/portfolios/" + p.ID},
		{http.MethodGet, "/api/portfolios/" + p.ID + "/stats"},
		{http.MethodPost, "/api/portfolios/" + p.ID + "/assets"},
		{http.MethodPut, "/api/portfolios/" + p.ID + "/assets/x"},
		{http.MethodDelete, "/api/portfolios/" + p.ID + "/assets/x"},
	} {
		w := ts.do(tc.method, tc.path, bob, gin.H{"name": "hijack"})
		assert.Equal(t, http.StatusNotFound, w.Code, "%s %s", tc.method, tc.path)
	}

	w := ts.do(http.MethodGet, "/api/portfolios", bob, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 0, decode[map[string]any](t, w)["total"])
}

func TestListPortfoliosPagination(t *testing.T) {
	ts := newTestServer(t, nil)
	token := ts.login("pager@example.com")
	for i := 1; i <= 5; i++ {
		ts.createPortfolio(token, fmt.Sprintf("Portfolio %d", i))
	}

	type page struct {
		Items  []domain.Portfolio `json:"items"`
		Total  int64              `json:"total"`
		Limit  int                `json:"limit"`
		Offset int                `json:"offset"`
	}

	w := ts.do(http.MethodGet, "/api/portfolios", token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	got := decode[page](t, w)
	assert.Len(t, got.Items, 5)
	assert.EqualValues(t, 5, got.Total)
	assert.Equal(t, store.DefaultLimit, got.Limit)
	assert.NotContains(t, w.Body.String(), `"assets"`, "list omits assets")

	w = ts.do(http.MethodGet, "/api/portfolios?limit=2&offset=4", token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	got = decode[page](t, w)
	assert.Len(t, got.Items, 1)
	assert.Equal(t, 2, got.Limit)
	assert.Equal(t, 4, got.Offset)

	w = ts.do(http.MethodGet, "/api/portfolios?limit=100000", token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, store.MaxLimit, decode[page](t, w).Limit, "limit is clamped")

	for _, q := range []string{"limit=abc", "offset=-1", "limit=1.5"} {
		w = ts.do(http.MethodGet, "/api/portfolios?"+q, token, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
}

// ---- assets and stats -----------------------------------------------------

func TestAssetLifecycleAndStats(t *testing.T) {
	ts := newTestServer(t, nil)
	token := ts.login("investor@example.com")
	p := ts.createPortfolio(token, "Tech")

	aapl := ts.addAsset(token, p.ID, " aapl ", "10", "100", "150")
	assert.Equal(t, "AAPL", aapl.Symbol, "symbol is trimmed and upper-cased")
	assert.Equal(t, "10", aapl.Quantity.String())
	ts.addAsset(token, p.ID, "GOOGL", "5", "200", "250")

	// Numbers and strings are both accepted on input; output is always a string.
	w := ts.do(http.MethodPost, "/api/portfolios/"+p.ID+"/assets", token,
		`{"symbol":"MSFT","quantity":0.5,"purchase_price":"300.25","current_price":310}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"purchase_price":"300.25"`)

	// Validation
	w = ts.do(http.MethodPost, "/api/portfolios/"+p.ID+"/assets", token,
		gin.H{"symbol": "BAD", "quantity": "0", "purchase_price": "-1", "current_price": "1.123456789"})
	require.Equal(t, http.StatusBadRequest, w.Code)
	e := decode[errBody](t, w)
	assert.Len(t, e.Details, 3, "%v", e.Details)
	w = ts.do(http.MethodPost, "/api/portfolios/"+p.ID+"/assets", token, gin.H{"symbol": "", "quantity": "1"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = ts.do(http.MethodPost, "/api/portfolios/"+p.ID+"/assets", token, gin.H{"symbol": "X", "quantity": "1e20", "purchase_price": "1", "current_price": "1"})
	assert.Equal(t, http.StatusBadRequest, w.Code, "magnitude beyond NUMERIC(24,8) is rejected")

	// Stats: exact decimal arithmetic.
	w = ts.do(http.MethodGet, "/api/portfolios/"+p.ID+"/stats", token, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	stats := decode[domain.Stats](t, w)
	assert.Equal(t, 3, stats.AssetCount)
	assert.Equal(t, "2905", stats.TotalValue.String())    // 1500 + 1250 + 155
	assert.Equal(t, "2150.125", stats.TotalCost.String()) // 1000 + 1000 + 150.125
	assert.Equal(t, "754.875", stats.TotalGain.String())  // 2905 - 2150.125
	assert.Equal(t, "35.1084", stats.TotalGainPercent.String())
	assert.Equal(t, "155", stats.AssetBreakdown["MSFT"].String())

	// Update: only the supplied field changes.
	w = ts.do(http.MethodPut, "/api/portfolios/"+p.ID+"/assets/"+aapl.ID, token, gin.H{"current_price": "200"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := decode[domain.Asset](t, w)
	assert.Equal(t, "200", updated.CurrentPrice.String())
	assert.Equal(t, "100", updated.PurchasePrice.String())
	assert.True(t, updated.Quantity.Equal(decimal.NewFromInt(10)))

	w = ts.do(http.MethodPut, "/api/portfolios/"+p.ID+"/assets/"+aapl.ID, token, gin.H{})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = ts.do(http.MethodPut, "/api/portfolios/"+p.ID+"/assets/"+aapl.ID, token, gin.H{"quantity": "-3"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = ts.do(http.MethodPut, "/api/portfolios/"+p.ID+"/assets/missing", token, gin.H{"quantity": "1"})
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Delete
	w = ts.do(http.MethodDelete, "/api/portfolios/"+p.ID+"/assets/"+aapl.ID, token, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = ts.do(http.MethodDelete, "/api/portfolios/"+p.ID+"/assets/"+aapl.ID, token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = ts.do(http.MethodGet, "/api/portfolios/"+p.ID, token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, decode[domain.Portfolio](t, w).Assets, 2)
}

// ---- plumbing -------------------------------------------------------------

func TestHealthRequestIDAndRouting(t *testing.T) {
	ts := newTestServer(t, nil)

	w := ts.do(http.MethodGet, "/health", "", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"), "a request id is always issued")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "client-supplied-123")
	w = httptest.NewRecorder()
	ts.handler.ServeHTTP(w, req)
	assert.Equal(t, "client-supplied-123", w.Header().Get("X-Request-ID"), "well-formed client ids are echoed")

	req.Header.Set("X-Request-ID", "has control\x01chars")
	w = httptest.NewRecorder()
	ts.handler.ServeHTTP(w, req)
	assert.NotEqual(t, "has control\x01chars", w.Header().Get("X-Request-ID"), "junk ids are replaced")

	w = ts.do(http.MethodGet, "/nope", "", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "route not found", decode[errBody](t, w).Error)

	w = ts.do(http.MethodPatch, "/health", "", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestNewPanicsWithoutDependencies(t *testing.T) {
	assert.Panics(t, func() { httpapi.New(httpapi.Options{}) })
}
