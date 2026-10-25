// Package auth implements password hashing and JWT issuance/verification.
package auth

import (
	"sync"

	"golang.org/x/crypto/bcrypt"
)

// MaxPasswordLength is bcrypt's hard input limit in bytes; longer inputs are
// rejected rather than silently truncated.
const MaxPasswordLength = 72

// PasswordHasher hashes and verifies passwords with bcrypt at a fixed cost.
// The zero value uses bcrypt.DefaultCost; tests lower Cost to bcrypt.MinCost.
type PasswordHasher struct {
	Cost int

	dummyOnce sync.Once
	dummy     string
}

func (h *PasswordHasher) cost() int {
	if h.Cost == 0 {
		return bcrypt.DefaultCost
	}
	return h.Cost
}

// Hash returns a bcrypt hash of password.
func (h *PasswordHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost())
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Check reports whether password matches the bcrypt hash.
func (h *PasswordHasher) Check(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CheckDummy performs a comparison against a throwaway hash at the same cost
// as real hashes. Callers use it when no account matched, so a login attempt
// takes the same time whether or not the email exists.
func (h *PasswordHasher) CheckDummy(password string) {
	h.dummyOnce.Do(func() {
		d, err := h.Hash("not-a-real-password")
		if err != nil {
			panic(err) // only possible with an out-of-range cost, a programmer error
		}
		h.dummy = d
	})
	h.Check(password, h.dummy)
}
