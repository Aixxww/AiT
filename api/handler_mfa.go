package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Aixxww/AiT/auth"
	"github.com/Aixxww/AiT/store"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// maxMFAAttempts caps wrong-code guesses on a single challenge. A challenge
// is only discarded once it is redeemed, expires, or reaches this limit.
const maxMFAAttempts = 5

type mfaLoginChallenge struct {
	UserID, Email string
	ExpiresAt     time.Time
	Attempts      int
}

var mfaChallenges = struct {
	sync.Mutex
	m map[string]mfaLoginChallenge
}{m: make(map[string]mfaLoginChallenge)}

func newMFASecret() (string, error) {
	b := make([]byte, 20)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}
func validTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	key, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if e != nil {
		return false
	}
	for _, offset := range []int64{-1, 0, 1} {
		c := uint64(now.Unix()/30 + offset)
		msg := make([]byte, 8)
		for i := 7; i >= 0; i-- {
			msg[i] = byte(c)
			c >>= 8
		}
		h := hmac.New(sha1.New, key)
		h.Write(msg)
		sum := h.Sum(nil)
		o := sum[len(sum)-1] & 15
		n := (uint32(sum[o])&127)<<24 | uint32(sum[o+1])<<16 | uint32(sum[o+2])<<8 | uint32(sum[o+3])
		if fmt.Sprintf("%06d", n%1000000) == code {
			return true
		}
	}
	return false
}
func (s *Server) mfaConfig(userID string) (*store.MFAConfig, error) {
	c, e := s.store.MFA().Get(userID)
	if e == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return c, e
}
func (s *Server) decryptMFASecret(c *store.MFAConfig) (string, error) {
	return s.cryptoHandler.cryptoService.DecryptFromStorage(c.SecretEncrypted, "mfa", c.UserID)
}
func (s *Server) handleMFAStatus(c *gin.Context) {
	cfg, e := s.mfaConfig(c.GetString("user_id"))
	if e != nil {
		c.JSON(500, gin.H{"error": "Failed to load MFA status"})
		return
	}
	c.JSON(200, gin.H{"enabled": cfg != nil && cfg.Enabled})
}
func (s *Server) handleMFASetup(c *gin.Context) {
	secret, e := newMFASecret()
	if e != nil {
		c.JSON(500, gin.H{"error": "Failed to create MFA secret"})
		return
	}
	email := c.GetString("email")
	uri := "otpauth://totp/AiT:" + email + "?secret=" + secret + "&issuer=AiT&algorithm=SHA1&digits=6&period=30"
	c.JSON(200, gin.H{"secret": secret, "otpauth_uri": uri, "issuer": "AiT"})
}
func (s *Server) handleMFAEnable(c *gin.Context) {
	var r struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}
	if c.ShouldBindJSON(&r) != nil || !validTOTP(r.Secret, r.Code, time.Now()) {
		c.JSON(400, gin.H{"error": "Invalid authenticator code"})
		return
	}
	uid := c.GetString("user_id")
	enc, e := s.cryptoHandler.cryptoService.EncryptForStorage(r.Secret, "mfa", uid)
	if e != nil {
		c.JSON(500, gin.H{"error": "Failed to protect MFA secret"})
		return
	}
	raw := make([]string, 10)
	hashes := make([]string, 10)
	for i := range raw {
		b := make([]byte, 8)
		rand.Read(b)
		raw[i] = strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
		h, _ := bcrypt.GenerateFromPassword([]byte(raw[i]), bcrypt.DefaultCost)
		hashes[i] = string(h)
	}
	data, _ := json.Marshal(hashes)
	now := time.Now().UTC()
	cfg := &store.MFAConfig{UserID: uid, SecretEncrypted: enc, RecoveryCodesJSON: string(data), Enabled: true, EnabledAt: now}
	if e = s.store.MFA().Save(cfg); e != nil {
		c.JSON(500, gin.H{"error": "Failed to enable MFA"})
		return
	}
	token, e := auth.GenerateJWT(uid, c.GetString("email"))
	if e != nil {
		c.JSON(500, gin.H{"error": "Failed to refresh session"})
		return
	}
	c.JSON(200, gin.H{"message": "MFA enabled", "recovery_codes": raw, "token": token})
}
func (s *Server) handleMFADisable(c *gin.Context) {
	var r struct {
		Code string `json:"code"`
	}
	if c.ShouldBindJSON(&r) != nil {
		c.JSON(400, gin.H{"error": "Code is required"})
		return
	}
	uid := c.GetString("user_id")
	cfg, e := s.mfaConfig(uid)
	if e != nil || cfg == nil || !cfg.Enabled {
		c.JSON(400, gin.H{"error": "MFA is not enabled"})
		return
	}
	secret, e := s.decryptMFASecret(cfg)
	if e != nil || !validTOTP(secret, r.Code, time.Now()) {
		c.JSON(400, gin.H{"error": "Invalid authenticator code"})
		return
	}
	if e = s.store.MFA().Disable(uid); e != nil {
		c.JSON(500, gin.H{"error": "Failed to disable MFA"})
		return
	}
	c.JSON(200, gin.H{"message": "MFA disabled"})
}
func (s *Server) createMFAChallenge(uid, email string) string {
	t := uuid.NewString() + base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()))
	now := time.Now()
	mfaChallenges.Lock()
	defer mfaChallenges.Unlock()
	// Opportunistically evict expired challenges so the map cannot grow forever.
	for k, v := range mfaChallenges.m {
		if now.After(v.ExpiresAt) {
			delete(mfaChallenges.m, k)
		}
	}
	mfaChallenges.m[t] = mfaLoginChallenge{UserID: uid, Email: email, ExpiresAt: now.Add(5 * time.Minute)}
	return t
}

// dropMFAChallenge removes a challenge, e.g. once it has been redeemed.
func dropMFAChallenge(token string) {
	mfaChallenges.Lock()
	delete(mfaChallenges.m, token)
	mfaChallenges.Unlock()
}

// bumpMFAChallengeAttempts records a wrong code. It reports the new attempt
// count and whether the challenge has been exhausted (and dropped).
func bumpMFAChallengeAttempts(token string) (int, bool) {
	mfaChallenges.Lock()
	defer mfaChallenges.Unlock()
	ch, ok := mfaChallenges.m[token]
	if !ok {
		return maxMFAAttempts, true
	}
	ch.Attempts++
	if ch.Attempts >= maxMFAAttempts {
		delete(mfaChallenges.m, token)
		return ch.Attempts, true
	}
	mfaChallenges.m[token] = ch
	return ch.Attempts, false
}
func (s *Server) handleMFALoginVerify(c *gin.Context) {
	var r struct {
		MFAToken string `json:"mfa_token"`
		Code     string `json:"code"`
	}
	if c.ShouldBindJSON(&r) != nil {
		c.JSON(400, gin.H{"error": "Verification code is required"})
		return
	}
	// Peek only: the challenge must survive a wrong code so a typo does not
	// force the user back through the password step.
	mfaChallenges.Lock()
	ch, ok := mfaChallenges.m[r.MFAToken]
	expired := ok && time.Now().After(ch.ExpiresAt)
	if expired {
		delete(mfaChallenges.m, r.MFAToken)
	}
	mfaChallenges.Unlock()
	if !ok || expired {
		c.JSON(401, gin.H{"error": "MFA verification expired; sign in again", "reason": "expired"})
		return
	}
	cfg, e := s.mfaConfig(ch.UserID)
	if e != nil || cfg == nil || !cfg.Enabled {
		dropMFAChallenge(r.MFAToken)
		c.JSON(401, gin.H{"error": "MFA is not available", "reason": "unavailable"})
		return
	}
	secret, e := s.decryptMFASecret(cfg)
	valid := e == nil && validTOTP(secret, r.Code, time.Now())
	if !valid {
		var hs []string
		_ = json.Unmarshal([]byte(cfg.RecoveryCodesJSON), &hs)
		for i, h := range hs {
			if bcrypt.CompareHashAndPassword([]byte(h), []byte(strings.ToUpper(strings.TrimSpace(r.Code)))) == nil {
				hs = append(hs[:i], hs[i+1:]...)
				b, _ := json.Marshal(hs)
				cfg.RecoveryCodesJSON = string(b)
				_ = s.store.MFA().Save(cfg)
				valid = true
				break
			}
		}
	}
	if !valid {
		// Cap the guesses so the six-digit code cannot be brute-forced, but
		// let a simple typo retry without restarting the whole login.
		attempts, exhausted := bumpMFAChallengeAttempts(r.MFAToken)
		if exhausted {
			c.JSON(429, gin.H{"error": "Too many invalid codes; sign in again", "reason": "too_many_attempts", "attempts_left": 0})
			return
		}
		c.JSON(401, gin.H{"error": "Invalid authenticator or recovery code", "reason": "invalid_code", "attempts_left": maxMFAAttempts - attempts})
		return
	}
	dropMFAChallenge(r.MFAToken)
	token, e := auth.GenerateJWT(ch.UserID, ch.Email)
	if e != nil {
		c.JSON(500, gin.H{"error": "Failed to create session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user_id": ch.UserID, "email": ch.Email, "message": "Login successful"})
}
