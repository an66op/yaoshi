package utils

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTValidHS256(t *testing.T) {
	InitJWT("test-secret-that-is-long-enough-for-hs256", 90)
	issuedAt := time.Now().UTC()
	token, err := GenerateToken(42, 7)
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
	if claims.AuthVersion != 7 {
		t.Fatalf("AuthVersion = %d, want 7", claims.AuthVersion)
	}
	if claims.Issuer != JWTIssuer {
		t.Fatalf("Issuer = %q, want %q", claims.Issuer, JWTIssuer)
	}
	if claims.Subject != "42" {
		t.Fatalf("Subject = %q, want 42", claims.Subject)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt is missing")
	}
	lifetime := claims.ExpiresAt.Time.Sub(issuedAt)
	if lifetime < 89*time.Second || lifetime > 91*time.Second {
		t.Fatalf("token lifetime = %s, want about 90s", lifetime)
	}
}

func TestJWTRejectsUnexpectedSigningMethod(t *testing.T) {
	secret := "test-secret-that-is-long-enough-for-hs256"
	InitJWT(secret, 3600)
	claims := &Claims{
		UserID:      42,
		AuthVersion: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    JWTIssuer,
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

func TestJWTRejectsWrongIssuer(t *testing.T) {
	secret := "test-secret-that-is-long-enough-for-hs256"
	InitJWT(secret, 3600)
	claims := &Claims{
		UserID:      42,
		AuthVersion: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "another-service",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	if _, err := ParseToken(token); err == nil {
		t.Fatal("ParseToken accepted a token from another issuer")
	}
}

func TestJWTRequiresInitialization(t *testing.T) {
	previousSecret := jwtSecret
	previousExpiration := jwtExpiration
	jwtSecret = nil
	jwtExpiration = 0
	t.Cleanup(func() {
		jwtSecret = previousSecret
		jwtExpiration = previousExpiration
	})
	if _, err := GenerateToken(1, 1); err == nil {
		t.Fatal("GenerateToken succeeded without an initialized secret")
	}
	if _, err := ParseToken("not-a-token"); err == nil {
		t.Fatal("ParseToken succeeded without an initialized secret")
	}
}

func TestJWTRejectsInvalidCredentialState(t *testing.T) {
	InitJWT("test-secret-that-is-long-enough-for-hs256", 3600)
	if _, err := GenerateToken(0, 1); err == nil {
		t.Fatal("GenerateToken accepted a zero user id")
	}
	if _, err := GenerateToken(1, 0); err == nil {
		t.Fatal("GenerateToken accepted a zero auth version")
	}
}
