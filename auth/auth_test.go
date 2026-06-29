package auth

import (
	"testing"
	"time"
)

func init() {
	// Set a stable JWT secret for tests.
	SetJWTSecret("test-jwt-secret-for-unit-tests")
}

// ---------------------------------------------------------------------------
// HashPassword / CheckPassword
// ---------------------------------------------------------------------------

func TestHashPassword(t *testing.T) {
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
}

func TestCheckPassword_Correct(t *testing.T) {
	hash, _ := HashPassword("correct-password")
	if !CheckPassword("correct-password", hash) {
		t.Fatal("expected password to match")
	}
}

func TestCheckPassword_Wrong(t *testing.T) {
	hash, _ := HashPassword("correct-password")
	if CheckPassword("wrong-password", hash) {
		t.Fatal("expected password to NOT match")
	}
}

// ---------------------------------------------------------------------------
// GenerateJWT / ValidateJWT
// ---------------------------------------------------------------------------

func TestGenerateAndValidateJWT(t *testing.T) {
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
}

func TestValidateJWT_InvalidToken(t *testing.T) {
	_, err := ValidateJWT("not-a-valid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	token, _ := GenerateJWT("user-1", "a@b.com")

	// Temporarily swap the secret
	orig := JWTSecret
	SetJWTSecret("different-secret")
	defer func() { JWTSecret = orig }()

	_, err := ValidateJWT(token)
	if err == nil {
		t.Fatal("expected error with wrong secret")
	}
}

// ---------------------------------------------------------------------------
// SetJWTSecret
// ---------------------------------------------------------------------------

func TestSetJWTSecret(t *testing.T) {
	SetJWTSecret("new-secret")
	if string(JWTSecret) != "new-secret" {
		t.Fatalf("expected %q, got %q", "new-secret", string(JWTSecret))
	}
	// Restore
	SetJWTSecret("test-jwt-secret-for-unit-tests")
}

// ---------------------------------------------------------------------------
// Token Blacklist
// ---------------------------------------------------------------------------

func TestBlacklistToken_IsBlacklisted(t *testing.T) {
	token := "token-to-blacklist"
	exp := time.Now().Add(1 * time.Hour)
	BlacklistToken(token, exp)

	if !IsTokenBlacklisted(token) {
		t.Fatal("expected token to be blacklisted")
	}
}

func TestIsTokenBlacklisted_NotPresent(t *testing.T) {
	if IsTokenBlacklisted("never-seen-this-token") {
		t.Fatal("expected token to NOT be blacklisted")
	}
}

func TestIsTokenBlacklisted_Expired(t *testing.T) {
	token := "expired-token"
	// Already expired
	BlacklistToken(token, time.Now().Add(-1*time.Second))

	if IsTokenBlacklisted(token) {
		t.Fatal("expected expired token to be removed from blacklist")
	}
}

func TestBlacklistToken_CapacitySweep(t *testing.T) {
	// Add many tokens to trigger sweep logic
	base := time.Now().Add(-1 * time.Second) // expired

	tokenBlacklist.Lock()
	for i := 0; i < maxBlacklistEntries+10; i++ {
		tokenBlacklist.items["sweep-token-"+time.Now().String()+string(rune(i))] = base
	}
	tokenBlacklist.Unlock()

	// Adding a new token should trigger sweep
	BlacklistToken("fresh-token", time.Now().Add(1*time.Hour))

	if !IsTokenBlacklisted("fresh-token") {
		t.Fatal("fresh token should survive sweep")
	}
}
