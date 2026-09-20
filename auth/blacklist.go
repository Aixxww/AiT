package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sync"
	"time"
)

// maxBlacklistEntries is the soft capacity threshold for the in-memory blacklist.
const maxBlacklistEntries = 100_000

// tokenBlacklist stores hashed tokens until their JWT expiration.
// Keys are SHA-256 hex digests of the raw token so full JWTs are not retained.
var tokenBlacklist = struct {
	sync.RWMutex
	items map[string]time.Time
}{items: make(map[string]time.Time)}

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// BlacklistToken adds token to the blacklist until expiration.
func BlacklistToken(token string, exp time.Time) {
	if token == "" {
		return
	}
	key := tokenKey(token)

	tokenBlacklist.Lock()
	defer tokenBlacklist.Unlock()

	tokenBlacklist.items[key] = exp

	if len(tokenBlacklist.items) <= maxBlacklistEntries {
		return
	}

	sweepExpiredLocked(time.Now())
	if len(tokenBlacklist.items) > maxBlacklistEntries {
		log.Printf("auth: token blacklist size (%d) exceeds limit (%d) after sweep; consider reducing JWT TTL or using a shared persistent store",
			len(tokenBlacklist.items), maxBlacklistEntries)
	}
}

// IsTokenBlacklisted reports whether token is currently blacklisted.
// Expired entries are removed lazily.
func IsTokenBlacklisted(token string) bool {
	if token == "" {
		return false
	}
	key := tokenKey(token)

	tokenBlacklist.Lock()
	defer tokenBlacklist.Unlock()

	exp, ok := tokenBlacklist.items[key]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(tokenBlacklist.items, key)
		return false
	}
	return true
}

// BlacklistSize returns the number of entries currently stored (including not-yet-swept expired ones).
// Intended for tests and diagnostics.
func BlacklistSize() int {
	tokenBlacklist.RLock()
	defer tokenBlacklist.RUnlock()
	return len(tokenBlacklist.items)
}

// ClearBlacklist removes all blacklist entries. Intended for tests.
func ClearBlacklist() {
	tokenBlacklist.Lock()
	defer tokenBlacklist.Unlock()
	tokenBlacklist.items = make(map[string]time.Time)
}

func sweepExpiredLocked(now time.Time) {
	for t, e := range tokenBlacklist.items {
		if now.After(e) {
			delete(tokenBlacklist.items, t)
		}
	}
}
