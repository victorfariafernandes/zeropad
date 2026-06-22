package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"zeropad-backend/adapters/db"
)

type contextKey string

const ClaimsKey contextKey = "claims"

func ClaimsFromContext(ctx context.Context) Claims {
	c, _ := ctx.Value(ClaimsKey).(Claims)
	return c
}

func ContextWithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, ClaimsKey, c)
}

const jwtTTL = 30 * 24 * time.Hour

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func IssueToken(secret []byte, user db.User) (string, error) {
	now := time.Now()
	claims := Claims{
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(jwtTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

func VerifyToken(secret []byte, raw string) (Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return Claims{}, fmt.Errorf("invalid token")
	}
	return *claims, nil
}
