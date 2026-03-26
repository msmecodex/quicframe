package middleware

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	qf "github.com/twaritam/quicframe"
	"github.com/twaritam/quicframe/protocol"
)

const jwtLocalsKey = "jwt_claims"

// JWTConfig controls JWT middleware behaviour.
type JWTConfig struct {
	// SigningKey is the HMAC-SHA256 secret (for HS256) or *rsa.PublicKey / *ecdsa.PublicKey.
	SigningKey interface{}

	// SigningMethod defaults to jwt.SigningMethodHS256.
	SigningMethod jwt.SigningMethod

	// TokenHeader is the request header that carries the Bearer token.
	// Defaults to "authorization".
	TokenHeader string

	// ContextKey is the locals key used to store the parsed *jwt.Token.
	// Defaults to "jwt_claims".
	ContextKey string

	// Optional custom claims factory.  Return a jwt.Claims implementation
	// to have the middleware populate it.  Defaults to jwt.MapClaims.
	NewClaims func() jwt.Claims
}

func (c *JWTConfig) setDefaults() {
	if c.SigningMethod == nil {
		c.SigningMethod = jwt.SigningMethodHS256
	}
	if c.TokenHeader == "" {
		c.TokenHeader = "authorization"
	}
	if c.ContextKey == "" {
		c.ContextKey = jwtLocalsKey
	}
	if c.NewClaims == nil {
		c.NewClaims = func() jwt.Claims { return jwt.MapClaims{} }
	}
}

// JWT returns middleware that validates a JWT Bearer token found in the
// named request header.  On success the parsed *jwt.Token is stored in
// ctx locals under cfg.ContextKey.
func JWT(cfg JWTConfig) qf.MiddlewareFunc {
	cfg.setDefaults()

	return func(next qf.HandlerFunc) qf.HandlerFunc {
		return func(ctx *qf.Context) error {
			raw := ctx.Header(cfg.TokenHeader)
			if raw == "" {
				return ctx.Error(protocol.StatusUnauthorized, "missing authorization header")
			}

			const prefix = "Bearer "
			if !strings.HasPrefix(raw, prefix) {
				return ctx.Error(protocol.StatusUnauthorized, "authorization header must be Bearer token")
			}
			tokenStr := raw[len(prefix):]

			claims := cfg.NewClaims()
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				if t.Method.Alg() != cfg.SigningMethod.Alg() {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return cfg.SigningKey, nil
			})
			if err != nil || !token.Valid {
				return ctx.Error(protocol.StatusUnauthorized, "invalid or expired token")
			}

			ctx.Set(cfg.ContextKey, token)
			return next(ctx)
		}
	}
}

// JWTSimple is a convenience wrapper for HMAC-SHA256 JWT validation.
//
//	mw := middleware.JWTSimple([]byte("my-secret"))
func JWTSimple(secret []byte) qf.MiddlewareFunc {
	return JWT(JWTConfig{
		SigningKey:    secret,
		SigningMethod: jwt.SigningMethodHS256,
	})
}

// GetClaims extracts the JWT MapClaims stored by the JWT middleware.
// Returns nil if the middleware was not applied or claims are a custom type.
func GetClaims(ctx *qf.Context) jwt.MapClaims {
	v, ok := ctx.Get(jwtLocalsKey)
	if !ok {
		return nil
	}
	token, ok := v.(*jwt.Token)
	if !ok {
		return nil
	}
	claims, _ := token.Claims.(jwt.MapClaims)
	return claims
}
