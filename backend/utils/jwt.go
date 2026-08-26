package utils

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

type Claims struct {
	UserID uint64 `json:"user_id"`
	jwt.RegisteredClaims
}

func InitJWT(secret string) {
	jwtSecret = []byte(secret)
}

func GenerateToken(userID uint64, expireHours int) (string, error) {
	if len(jwtSecret) == 0 {
		return "", errors.New("JWT 密钥尚未初始化")
	}
	if expireHours <= 0 {
		return "", errors.New("JWT 过期时间必须大于 0")
	}
	expirationTime := time.Now().Add(time.Duration(expireHours) * time.Hour)

	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
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
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("JWT 无效")
}
