package auth

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestMain(m *testing.M) {
	SetJWTSecret("test-jwt-secret-for-unit-tests")
	SetTokenTTL(DefaultTokenTTL)
	ClearBlacklist()
	m.Run()
}

func resetAuthState(t *testing.T) {
	t.Helper()
	SetJWTSecret("test-jwt-secret-for-unit-tests")
	SetTokenTTL(DefaultTokenTTL)
	ClearBlacklist()
}

// ---------------------------------------------------------------------------
// HashPassword / CheckPassword
// ---------------------------------------------------------------------------

func TestHashPassword(t *testing.T) {
	resetAuthState(t)

	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == "hunter2" {
		t.Fatal("hash should differ from plaintext")
	}
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") {
		t.Fatalf("expected bcrypt hash prefix, got %q", hash[:4])
	}
}

func TestHashPassword_Empty(t *testing.T) {
	_, err := HashPassword("")
	if !errors.Is(err, ErrEmptyPassword) {
		t.Fatalf("expected ErrEmptyPassword, got %v", err)
	}
}

func TestHashPassword_UniqueSalts(t *testing.T) {
	h1, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("expected different hashes due to unique salts")
	}
	if !CheckPassword("same-password", h1) || !CheckPassword("same-password", h2) {
		t.Fatal("both hashes should verify")
	}
}

func TestCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		password string
		hash     string
		want     bool
	}{
		{"correct", "correct-password", hash, true},
		{"wrong", "wrong-password", hash, false},
		{"empty password", "", hash, false},
		{"empty hash", "correct-password", "", false},
		{"both empty", "", "", false},
		{"garbage hash", "correct-password", "not-a-bcrypt-hash", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckPassword(tt.password, tt.hash); got != tt.want {
				t.Fatalf("CheckPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckPassword_Unicode(t *testing.T) {
	pw := "密码🔒café"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(pw, hash) {
		t.Fatal("unicode password should verify")
	}
	if CheckPassword("密码🔒cafe", hash) {
		t.Fatal("near-miss unicode should not verify")
	}
}

// ---------------------------------------------------------------------------
// GenerateJWT / ValidateJWT
// ---------------------------------------------------------------------------

func TestGenerateAndValidateJWT(t *testing.T) {
	resetAuthState(t)

	token, err := GenerateJWT("user-123", "test@example.com")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := ValidateJWT(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Fatalf("UserID = %q, want %q", claims.UserID, "user-123")
	}
	if claims.Email != "test@example.com" {
		t.Fatalf("Email = %q, want %q", claims.Email, "test@example.com")
	}
	if claims.Issuer != DefaultIssuer {
		t.Fatalf("Issuer = %q, want %q", claims.Issuer, DefaultIssuer)
	}
	if claims.Subject != "user-123" {
		t.Fatalf("Subject = %q, want %q", claims.Subject, "user-123")
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil || claims.NotBefore == nil {
		t.Fatal("expected registered time claims to be set")
	}
}

func TestGenerateJWT_EmptySecret(t *testing.T) {
	SetJWTSecret("")
	JWTSecret = nil
	jwtSecret = nil
	t.Cleanup(func() { resetAuthState(t) })

	_, err := GenerateJWT("u", "e@x.com")
	if !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("expected ErrEmptySecret, got %v", err)
	}
}

func TestValidateJWT_EmptyToken(t *testing.T) {
	resetAuthState(t)
	_, err := ValidateJWT("")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestValidateJWT_InvalidToken(t *testing.T) {
	resetAuthState(t)
	_, err := ValidateJWT("not-a-valid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	resetAuthState(t)
	token, err := GenerateJWT("user-1", "a@b.com")
	if err != nil {
		t.Fatal(err)
	}

	SetJWTSecret("different-secret")
	t.Cleanup(func() { resetAuthState(t) })

	_, err = ValidateJWT(token)
	if err == nil {
		t.Fatal("expected error with wrong secret")
	}
}

func TestValidateJWT_Expired(t *testing.T) {
	resetAuthState(t)

	token, err := GenerateJWTWithTTL("user-1", "a@b.com", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	_, err = ValidateJWT(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateJWT_TamperedPayload(t *testing.T) {
	resetAuthState(t)

	token, err := GenerateJWT("user-1", "a@b.com")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}
	// Flip a character in the payload segment.
	payload := []byte(parts[1])
	payload[len(payload)/2] ^= 0x1
	parts[1] = string(payload)
	tampered := strings.Join(parts, ".")

	_, err = ValidateJWT(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestValidateJWT_RejectsNonHMAC(t *testing.T) {
	resetAuthState(t)

	// Craft an unsigned "none" alg token — parser must reject it.
	claims := Claims{
		UserID: "attacker",
		Email:  "x@y.z",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Issuer:    DefaultIssuer,
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	raw, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("craft none token: %v", err)
	}

	_, err = ValidateJWT(raw)
	if err == nil {
		t.Fatal("expected rejection of alg=none token")
	}
}

func TestGenerateJWTWithTTL(t *testing.T) {
	resetAuthState(t)

	token, err := GenerateJWTWithTTL("u", "e@x.com", 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateJWT(token)
	if err != nil {
		t.Fatal(err)
	}
	remaining := time.Until(claims.ExpiresAt.Time)
	if remaining < time.Hour || remaining > 2*time.Hour+time.Minute {
		t.Fatalf("unexpected remaining TTL: %v", remaining)
	}
}

func TestSetTokenTTL(t *testing.T) {
	resetAuthState(t)
	defer resetAuthState(t)

	SetTokenTTL(0) // ignored
	if TokenTTL() != DefaultTokenTTL {
		t.Fatalf("zero TTL should be ignored, got %v", TokenTTL())
	}

	SetTokenTTL(-time.Hour) // ignored
	if TokenTTL() != DefaultTokenTTL {
		t.Fatalf("negative TTL should be ignored, got %v", TokenTTL())
	}

	SetTokenTTL(time.Hour)
	if TokenTTL() != time.Hour {
		t.Fatalf("TokenTTL = %v, want %v", TokenTTL(), time.Hour)
	}

	token, err := GenerateJWT("u", "e@x.com")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateJWT(token)
	if err != nil {
		t.Fatal(err)
	}
	remaining := time.Until(claims.ExpiresAt.Time)
	if remaining < 30*time.Minute || remaining > time.Hour+time.Minute {
		t.Fatalf("token should use configured TTL, remaining=%v", remaining)
	}
}

func TestSetJWTSecret(t *testing.T) {
	defer resetAuthState(t)

	SetJWTSecret("new-secret")
	secret, err := secretBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "new-secret" {
		t.Fatalf("expected %q, got %q", "new-secret", string(secret))
	}
	if string(JWTSecret) != "new-secret" {
		t.Fatalf("JWTSecret alias = %q, want %q", string(JWTSecret), "new-secret")
	}
}

func TestSecretBytes_LegacyJWTSecretAssignment(t *testing.T) {
	defer resetAuthState(t)

	secretMu.Lock()
	jwtSecret = nil
	JWTSecret = []byte("legacy-direct-secret")
	secretMu.Unlock()

	secret, err := secretBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "legacy-direct-secret" {
		t.Fatalf("got %q", string(secret))
	}
}

// ---------------------------------------------------------------------------
// Token Blacklist
// ---------------------------------------------------------------------------

func TestBlacklistToken_IsBlacklisted(t *testing.T) {
	resetAuthState(t)

	token := "token-to-blacklist"
	BlacklistToken(token, time.Now().Add(time.Hour))

	if !IsTokenBlacklisted(token) {
		t.Fatal("expected token to be blacklisted")
	}
	// Raw JWT must not be stored as map key.
	tokenBlacklist.RLock()
	_, rawStored := tokenBlacklist.items[token]
	tokenBlacklist.RUnlock()
	if rawStored {
		t.Fatal("blacklist must store hashed keys, not raw tokens")
	}
}

func TestIsTokenBlacklisted_NotPresent(t *testing.T) {
	resetAuthState(t)
	if IsTokenBlacklisted("never-seen-this-token") {
		t.Fatal("expected token to NOT be blacklisted")
	}
}

func TestIsTokenBlacklisted_Empty(t *testing.T) {
	resetAuthState(t)
	if IsTokenBlacklisted("") {
		t.Fatal("empty token should not be blacklisted")
	}
}

func TestBlacklistToken_EmptyIgnored(t *testing.T) {
	resetAuthState(t)
	BlacklistToken("", time.Now().Add(time.Hour))
	if BlacklistSize() != 0 {
		t.Fatal("empty token should not be inserted")
	}
}

func TestIsTokenBlacklisted_Expired(t *testing.T) {
	resetAuthState(t)

	token := "expired-token"
	BlacklistToken(token, time.Now().Add(-time.Second))

	if IsTokenBlacklisted(token) {
		t.Fatal("expected expired token to be removed from blacklist")
	}
	if BlacklistSize() != 0 {
		t.Fatalf("expected expired entry removed, size=%d", BlacklistSize())
	}
}

func TestBlacklist_RoundTripWithRealJWT(t *testing.T) {
	resetAuthState(t)

	token, err := GenerateJWT("user-9", "z@z.com")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateJWT(token)
	if err != nil {
		t.Fatal(err)
	}

	BlacklistToken(token, claims.ExpiresAt.Time)
	if !IsTokenBlacklisted(token) {
		t.Fatal("issued JWT should be blacklisted after logout")
	}

	// Validation still succeeds (middleware checks blacklist separately),
	// matching existing API server behavior.
	if _, err := ValidateJWT(token); err != nil {
		t.Fatalf("ValidateJWT should still parse blacklisted token: %v", err)
	}
}

func TestBlacklistToken_CapacitySweep(t *testing.T) {
	resetAuthState(t)
	defer resetAuthState(t)

	expired := time.Now().Add(-time.Second)
	tokenBlacklist.Lock()
	for i := 0; i < maxBlacklistEntries+10; i++ {
		tokenBlacklist.items[fmt.Sprintf("sweep-key-%d", i)] = expired
	}
	tokenBlacklist.Unlock()

	BlacklistToken("fresh-token", time.Now().Add(time.Hour))

	if !IsTokenBlacklisted("fresh-token") {
		t.Fatal("fresh token should survive sweep")
	}
	if BlacklistSize() > maxBlacklistEntries {
		t.Fatalf("after sweep size should be <= limit, got %d", BlacklistSize())
	}
}

func TestClearBlacklist(t *testing.T) {
	resetAuthState(t)
	BlacklistToken("a", time.Now().Add(time.Hour))
	BlacklistToken("b", time.Now().Add(time.Hour))
	ClearBlacklist()
	if BlacklistSize() != 0 {
		t.Fatal("expected empty blacklist")
	}
	if IsTokenBlacklisted("a") || IsTokenBlacklisted("b") {
		t.Fatal("cleared tokens should not remain blacklisted")
	}
}

func TestBlacklist_ConcurrentAccess(t *testing.T) {
	resetAuthState(t)

	const goroutines = 32
	const perG = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				tok := fmt.Sprintf("tok-%d-%d", id, i)
				BlacklistToken(tok, time.Now().Add(time.Hour))
				_ = IsTokenBlacklisted(tok)
				_ = BlacklistSize()
			}
		}(g)
	}
	wg.Wait()

	if BlacklistSize() != goroutines*perG {
		t.Fatalf("size = %d, want %d", BlacklistSize(), goroutines*perG)
	}
}

func TestTokenKey_Stable(t *testing.T) {
	a := tokenKey("abc")
	b := tokenKey("abc")
	c := tokenKey("abd")
	if a != b {
		t.Fatal("same token must hash identically")
	}
	if a == c {
		t.Fatal("different tokens must not collide (practically)")
	}
	if len(a) != 64 {
		t.Fatalf("sha256 hex length = %d, want 64", len(a))
	}
}
