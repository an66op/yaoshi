package utils

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTValidHS256(t *testing.T) {
	InitJWT("test-secret-that-is-long-enough-for-hs256")
	token, err := GenerateToken(42, 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.UserID != 42 {
		t.Fatalf("UserID = %d, want 42", claims.UserID)
	}
}

func TestJWTRejectsUnexpectedSigningMethod(t *testing.T) {
	secret := "test-secret-that-is-long-enough-for-hs256"
	InitJWT(secret)
	claims := &Claims{
		UserID: 42,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	if _, err := ParseToken(token); err == nil {
		t.Fatal("ParseToken accepted a token signed with HS384")
	}
}

func TestJWTRequiresInitialization(t *testing.T) {
	previous := jwtSecret
	jwtSecret = nil
	t.Cleanup(func() { jwtSecret = previous })
	if _, err := GenerateToken(1, 1); err == nil {
		t.Fatal("GenerateToken succeeded without an initialized secret")
	}
	if _, err := ParseToken("not-a-token"); err == nil {
		t.Fatal("ParseToken succeeded without an initialized secret")
	}
}
