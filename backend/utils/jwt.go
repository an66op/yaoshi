package utils

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const JWTIssuer = "wangzhe-backend"

var (
	jwtSecret     []byte
	jwtExpiration time.Duration
)

type Claims struct {
	UserID      uint64 `json:"user_id"`
	AuthVersion uint64 `json:"auth_version"`
	jwt.RegisteredClaims
}

// InitJWT configures token signing and lifetime. expireSeconds intentionally
// uses the same seconds unit as config.jwt.expire and BACKEND_JWT_EXPIRE.
func InitJWT(secret string, expireSeconds int) {
	jwtSecret = []byte(secret)
	jwtExpiration = time.Duration(expireSeconds) * time.Second
}

func GenerateToken(userID, authVersion uint64) (string, error) {
	if len(jwtSecret) == 0 {
		return "", errors.New("JWT 密钥尚未初始化")
	}
	if jwtExpiration <= 0 {
		return "", errors.New("JWT 过期时间必须大于 0")
	}
	if userID == 0 || authVersion == 0 {
		return "", errors.New("JWT 账号信息无效")
	}
	now := time.Now().UTC()
	expirationTime := now.Add(jwtExpiration)

	claims := &Claims{
		UserID:      userID,
		AuthVersion: authVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    JWTIssuer,
			Subject:   strconv.FormatUint(userID, 10),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func ParseToken(tokenString string) (*Claims, error) {
	if len(jwtSecret) == 0 {
		return nil, errors.New("JWT 密钥尚未初始化")
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("不允许的 JWT 签名算法: %s", token.Method.Alg())
		}
		return jwtSecret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(JWTIssuer),
	)

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("JWT 无效")
}
