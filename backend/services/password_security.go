package services

import "gorm.io/gorm"

// passwordSessionUpdate must be used for every password mutation. Keeping the
// hash and auth-version increment in one SQL UPDATE prevents a window where a
// new password is active while tokens from the old password remain valid.
func passwordSessionUpdate(hash string) map[string]any {
	return map[string]any{
		"password":     hash,
		"auth_version": gorm.Expr("auth_version + 1"),
	}
}
