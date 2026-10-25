package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MinSecretLength is the minimum HMAC key size accepted, in bytes. HS256 keys
// shorter than the hash output are weaker than the algorithm itself.
const MinSecretLength = 32

// ErrSecretTooShort is returned by NewTokenManager for an under-sized secret.
var ErrSecretTooShort = fmt.Errorf("jwt secret must be at least %d bytes", MinSecretLength)

// Claims is the typed payload carried by every token this service issues.
type Claims struct {
	// UserID duplicates Subject under an explicit name so callers never have
	// to remember which registered claim the service chose.
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

// TokenManager signs and verifies HS256 JWTs for one issuer.
type TokenManager struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

// NewTokenManager returns a manager that issues tokens valid for ttl.
func NewTokenManager(secret []byte, issuer string, ttl time.Duration) (*TokenManager, error) {
	if len(secret) < MinSecretLength {
		return nil, ErrSecretTooShort
	}
	if issuer == "" {
		return nil, errors.New("jwt issuer must not be empty")
	}
	if ttl <= 0 {
		return nil, errors.New("jwt ttl must be positive")
	}
	return &TokenManager{secret: secret, issuer: issuer, ttl: ttl, now: time.Now}, nil
}

// Issue returns a signed token identifying userID.
func (m *TokenManager) Issue(userID string) (string, error) {
	now := m.now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Verify parses and validates a token string and returns its claims. The
// signing method, signature, issuer, expiry and issued-at are all checked.
func (m *TokenManager) Verify(tokenString string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(tokenString, &claims,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return nil, err
	}
	if claims.UserID == "" || claims.UserID != claims.Subject {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return &claims, nil
}
