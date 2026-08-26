package utils

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const (
	// MinPasswordLength applies to newly created or changed credentials. Old
	// hashes remain valid so an upgrade never locks existing users out.
	MinPasswordLength = 8
	// bcrypt silently rejects inputs longer than 72 bytes. Rejecting them at
	// the boundary avoids an apparently successful password change that cannot
	// subsequently be verified.
	MaxPasswordLength = 72
)

// dummyPasswordHash is deliberately a real bcrypt hash. Authentication uses
// it when a username is absent so missing users and wrong passwords take the
// same expensive code path and cannot be distinguished by response timing.
const dummyPasswordHash = "$2y$10$fVYUr.QCkifAoj0detaC1OwipYvIAmKOPKFmf3uKqAdfusDpR.OYa"

func ValidatePassword(password string) error {
	length := len([]byte(password))
	if length < MinPasswordLength || length > MaxPasswordLength {
		return fmt.Errorf("password length must be between %d and %d bytes", MinPasswordLength, MaxPasswordLength)
	}
	return nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CheckMissingUserPassword consumes the same bcrypt work as a normal login.
// Its result must never be used for authentication.
func CheckMissingUserPassword(password string) {
	_ = bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte(password))
}
