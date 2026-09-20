package auth

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// DefaultTokenTTL is the default JWT lifetime.
	DefaultTokenTTL = 24 * time.Hour
	// DefaultIssuer is embedded in issued tokens.
	DefaultIssuer = "aitAI"
)

var (
	secretMu  sync.RWMutex
	jwtSecret []byte

	ttlMu    sync.RWMutex
	tokenTTL = DefaultTokenTTL
)

// JWTSecret is the raw signing key. Prefer SetJWTSecret / secretBytes for access;
// retained as an exported alias for compatibility with existing call sites and tests.
//
// Deprecated: use SetJWTSecret; direct assignment is not concurrency-safe.
var JWTSecret []byte

// Claims represents JWT claims for authenticated users.
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// SetJWTSecret sets the JWT HMAC secret key.
func SetJWTSecret(secret string) {
	secretMu.Lock()
	defer secretMu.Unlock()
	jwtSecret = []byte(secret)
	JWTSecret = jwtSecret
}

// SetTokenTTL configures the lifetime used by GenerateJWT. Non-positive values are ignored.
func SetTokenTTL(ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	ttlMu.Lock()
	defer ttlMu.Unlock()
	tokenTTL = ttl
}

// TokenTTL returns the configured JWT lifetime.
func TokenTTL() time.Duration {
	ttlMu.RLock()
	defer ttlMu.RUnlock()
	return tokenTTL
}

func secretBytes() ([]byte, error) {
	secretMu.RLock()
	defer secretMu.RUnlock()
	if len(jwtSecret) == 0 && len(JWTSecret) > 0 {
		// Honor legacy direct assignment to JWTSecret.
		return JWTSecret, nil
	}
	if len(jwtSecret) == 0 {
		return nil, ErrEmptySecret
	}
	out := make([]byte, len(jwtSecret))
	copy(out, jwtSecret)
	return out, nil
}

// GenerateJWT issues a signed HS256 token for the given user.
func GenerateJWT(userID, email string) (string, error) {
	return GenerateJWTWithTTL(userID, email, TokenTTL())
}

// GenerateJWTWithTTL issues a signed HS256 token with a custom lifetime.
func GenerateJWTWithTTL(userID, email string, ttl time.Duration) (string, error) {
	secret, err := secretBytes()
	if err != nil {
		return "", err
	}
	if ttl <= 0 {
		ttl = DefaultTokenTTL
	}

	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    DefaultIssuer,
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// ValidateJWT parses and validates a JWT token string.
func ValidateJWT(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, ErrInvalidToken
	}

	secret, err := secretBytes()
	if err != nil {
		return nil, err
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: %v", ErrUnexpectedSigningMethod, token.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
