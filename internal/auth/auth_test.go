package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func newManager(t *testing.T) *TokenManager {
	t.Helper()
	m, err := NewTokenManager(testSecret, "portfolio-manager-test", time.Hour)
	require.NoError(t, err)
	return m
}

func TestNewTokenManagerRejectsBadConfig(t *testing.T) {
	_, err := NewTokenManager([]byte("short"), "iss", time.Hour)
	assert.ErrorIs(t, err, ErrSecretTooShort)

	_, err = NewTokenManager(testSecret, "", time.Hour)
	assert.Error(t, err)

	_, err = NewTokenManager(testSecret, "iss", 0)
	assert.Error(t, err)
}

func TestIssueAndVerifyRoundTrip(t *testing.T) {
	m := newManager(t)
	tok, err := m.Issue("user-1")
	require.NoError(t, err)

	claims, err := m.Verify(tok)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.UserID)
	assert.Equal(t, "user-1", claims.Subject)
	assert.Equal(t, "portfolio-manager-test", claims.Issuer)
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	m := newManager(t)
	issuedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return issuedAt }
	tok, err := m.Issue("user-1")
	require.NoError(t, err)

	m.now = func() time.Time { return issuedAt.Add(2 * time.Hour) }
	_, err = m.Verify(tok)
	assert.ErrorIs(t, err, jwt.ErrTokenExpired)
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	m := newManager(t)
	tok, err := m.Issue("user-1")
	require.NoError(t, err)

	other, err := NewTokenManager([]byte(strings.Repeat("x", 32)), "portfolio-manager-test", time.Hour)
	require.NoError(t, err)
	_, err = other.Verify(tok)
	assert.ErrorIs(t, err, jwt.ErrTokenSignatureInvalid)
}

func TestVerifyRejectsWrongIssuer(t *testing.T) {
	m := newManager(t)
	other, err := NewTokenManager(testSecret, "someone-else", time.Hour)
	require.NoError(t, err)
	tok, err := other.Issue("user-1")
	require.NoError(t, err)

	_, err = m.Verify(tok)
	assert.ErrorIs(t, err, jwt.ErrTokenInvalidIssuer)
}

func TestVerifyRejectsOtherSigningMethods(t *testing.T) {
	m := newManager(t)
	claims := Claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "portfolio-manager-test",
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	// Same key, different HMAC variant: must be refused by WithValidMethods.
	hs512, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(testSecret)
	require.NoError(t, err)
	_, err = m.Verify(hs512)
	assert.ErrorIs(t, err, jwt.ErrTokenSignatureInvalid)

	// The classic "alg: none" bypass.
	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	_, err = m.Verify(none)
	assert.Error(t, err)
}

func TestVerifyRejectsMissingExpiry(t *testing.T) {
	m := newManager(t)
	claims := Claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "portfolio-manager-test",
			Subject:  "user-1",
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testSecret)
	require.NoError(t, err)
	_, err = m.Verify(tok)
	assert.ErrorIs(t, err, jwt.ErrTokenRequiredClaimMissing)
}

func TestVerifyRejectsSubjectMismatch(t *testing.T) {
	m := newManager(t)
	claims := Claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "portfolio-manager-test",
			Subject:   "someone-else",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(testSecret)
	require.NoError(t, err)
	_, err = m.Verify(tok)
	assert.ErrorIs(t, err, jwt.ErrTokenInvalidClaims)
}

func TestPasswordHashing(t *testing.T) {
	h := &PasswordHasher{Cost: bcrypt.MinCost}
	hash, err := h.Hash("correct horse battery staple")
	require.NoError(t, err)
	assert.NotEqual(t, "correct horse battery staple", hash)
	assert.True(t, h.Check("correct horse battery staple", hash))
	assert.False(t, h.Check("wrong", hash))
	assert.False(t, h.Check("anything", "not-a-bcrypt-hash"))
	h.CheckDummy("anything") // must not panic and must be callable repeatedly
	h.CheckDummy("anything")

	_, err = h.Hash(strings.Repeat("a", MaxPasswordLength+1))
	assert.Error(t, err, "bcrypt must refuse inputs over 72 bytes instead of truncating")

	var def PasswordHasher
	assert.Equal(t, bcrypt.DefaultCost, def.cost(), "zero value uses the default cost")
}
