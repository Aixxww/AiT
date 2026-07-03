package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"strings"
	"testing"
)

// helper: generate a test RSA key pair and return PEM-encoded private key.
func testRSAPrivateKeyPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

// helper: build a CryptoService with a fresh key pair and a random 32-byte data key.
func testCryptoService(t *testing.T) *CryptoService {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	dataKey := make([]byte, 32)
	if _, err := rand.Read(dataKey); err != nil {
		t.Fatalf("generate data key: %v", err)
	}
	return &CryptoService{
		privateKey: key,
		publicKey:  &key.PublicKey,
		dataKey:    dataKey,
	}
}

// ---------------------------------------------------------------------------
// ParseRSAPrivateKeyFromPEM
// ---------------------------------------------------------------------------

func TestParseRSAPrivateKeyFromPEM_PKCS1(t *testing.T) {
	pemBytes := testRSAPrivateKeyPEM(t)
	key, err := ParseRSAPrivateKeyFromPEM(pemBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key == nil {
		t.Fatal("expected non-nil key")
	}
}

func TestParseRSAPrivateKeyFromPEM_PKCS8(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	key, err := ParseRSAPrivateKeyFromPEM(pemBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key == nil {
		t.Fatal("expected non-nil key")
	}
}

func TestParseRSAPrivateKeyFromPEM_InvalidPEM(t *testing.T) {
	_, err := ParseRSAPrivateKeyFromPEM([]byte("not-a-pem"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestParseRSAPrivateKeyFromPEM_UnsupportedType(t *testing.T) {
	block := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("dummy")})
	_, err := ParseRSAPrivateKeyFromPEM(block)
	if err == nil {
		t.Fatal("expected error for unsupported key type")
	}
}

// ---------------------------------------------------------------------------
// normalizeAESKey
// ---------------------------------------------------------------------------

func TestNormalizeAESKey(t *testing.T) {
	tests := []struct {
		name   string
		len    int
		wantOK bool
		wantSz int
	}{
		{"empty", 0, false, 0},
		{"16 bytes", 16, true, 16},
		{"24 bytes", 24, true, 24},
		{"32 bytes", 32, true, 32},
		{"odd length - hashed to 32", 7, true, 32},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := make([]byte, tc.len)
			got, ok := normalizeAESKey(raw)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && len(got) != tc.wantSz {
				t.Fatalf("key length = %d, want %d", len(got), tc.wantSz)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// decodePossibleKey
// ---------------------------------------------------------------------------

func TestDecodePossibleKey_Base64_32Bytes(t *testing.T) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	key, ok := decodePossibleKey(encoded)
	if !ok {
		t.Fatal("expected ok")
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d, want 32", len(key))
	}
}

func TestDecodePossibleKey_Hex_32Bytes(t *testing.T) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	encoded := hex.EncodeToString(raw)
	key, ok := decodePossibleKey(encoded)
	if !ok {
		t.Fatal("expected ok")
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d, want 32", len(key))
	}
}

// ---------------------------------------------------------------------------
// EncryptForStorage / DecryptFromStorage
// ---------------------------------------------------------------------------

func TestEncryptDecryptForStorage(t *testing.T) {
	cs := testCryptoService(t)
	plaintext := "super-secret-api-key"

	encrypted, err := cs.EncryptForStorage(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(encrypted, storagePrefix) {
		t.Fatalf("expected prefix %q", storagePrefix)
	}

	decrypted, err := cs.DecryptFromStorage(encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptForStorage_EmptyString(t *testing.T) {
	cs := testCryptoService(t)
	enc, err := cs.EncryptForStorage("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc != "" {
		t.Fatalf("expected empty, got %q", enc)
	}
}

func TestEncryptForStorage_AlreadyEncrypted(t *testing.T) {
	cs := testCryptoService(t)
	already := storagePrefix + "abc:def"
	enc, err := cs.EncryptForStorage(already)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc != already {
		t.Fatalf("expected passthrough for already-encrypted value")
	}
}

func TestDecryptFromStorage_NotEncrypted(t *testing.T) {
	cs := testCryptoService(t)
	_, err := cs.DecryptFromStorage("plain-text")
	if err == nil {
		t.Fatal("expected error for non-encrypted value")
	}
}

func TestEncryptDecryptForStorage_WithAAD(t *testing.T) {
	cs := testCryptoService(t)
	plaintext := "aad-protected"
	aad := []string{"user-123", "session-abc"}

	encrypted, err := cs.EncryptForStorage(plaintext, aad...)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	decrypted, err := cs.DecryptFromStorage(encrypted, aad...)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}

	// Decryption with wrong AAD should fail
	_, err = cs.DecryptFromStorage(encrypted, "wrong-aad")
	if err == nil {
		t.Fatal("expected error with wrong AAD")
	}
}

func TestDecryptFromStorage_EmptyString(t *testing.T) {
	cs := testCryptoService(t)
	dec, err := cs.DecryptFromStorage("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != "" {
		t.Fatalf("expected empty, got %q", dec)
	}
}

func TestEncryptForStorage_NoDataKey(t *testing.T) {
	cs := &CryptoService{}
	_, err := cs.EncryptForStorage("secret")
	if err == nil {
		t.Fatal("expected error when data key is missing")
	}
}

// ---------------------------------------------------------------------------
// HasDataKey / IsEncryptedStorageValue
// ---------------------------------------------------------------------------

func TestHasDataKey(t *testing.T) {
	cs := testCryptoService(t)
	if !cs.HasDataKey() {
		t.Fatal("expected HasDataKey true")
	}
	empty := &CryptoService{}
	if empty.HasDataKey() {
		t.Fatal("expected HasDataKey false for empty service")
	}
}

func TestIsEncryptedStorageValue(t *testing.T) {
	cs := testCryptoService(t)
	if cs.IsEncryptedStorageValue("plain") {
		t.Fatal("expected false for plain text")
	}
	if !cs.IsEncryptedStorageValue(storagePrefix + "something") {
		t.Fatal("expected true for prefixed value")
	}
}

// ---------------------------------------------------------------------------
// GetPublicKeyPEM
// ---------------------------------------------------------------------------

func TestGetPublicKeyPEM(t *testing.T) {
	cs := testCryptoService(t)
	pem, err := cs.GetPublicKeyPEM()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(pem, "-----BEGIN PUBLIC KEY-----") {
		t.Fatalf("unexpected PEM: %s", pem[:40])
	}
}

// ---------------------------------------------------------------------------
// GenerateKeyPair / GenerateDataKey
// ---------------------------------------------------------------------------

func TestGenerateKeyPair(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(priv, "RSA PRIVATE KEY") {
		t.Fatal("expected RSA PRIVATE KEY in PEM")
	}
	if !strings.Contains(pub, "PUBLIC KEY") {
		t.Fatal("expected PUBLIC KEY in PEM")
	}
}

func TestGenerateDataKey(t *testing.T) {
	key, err := GenerateDataKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("key length = %d, want 32", len(raw))
	}
}

// ---------------------------------------------------------------------------
// composeAAD
// ---------------------------------------------------------------------------

func TestComposeAAD(t *testing.T) {
	if got := composeAAD(nil); got != nil {
		t.Fatalf("expected nil, got %q", got)
	}
	if got := composeAAD([]string{}); got != nil {
		t.Fatalf("expected nil for empty slice, got %q", got)
	}
	if got := string(composeAAD([]string{"a", "b"})); got != "a|b" {
		t.Fatalf("got %q, want %q", got, "a|b")
	}
}

// ---------------------------------------------------------------------------
// EncryptedString (Scan / Value)
// ---------------------------------------------------------------------------

func TestEncryptedString_ScanNil(t *testing.T) {
	var es EncryptedString
	if err := es.Scan(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if es != "" {
		t.Fatalf("expected empty, got %q", es)
	}
}

func TestEncryptedString_ScanString(t *testing.T) {
	var es EncryptedString
	if err := es.Scan("hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if es != "hello" {
		t.Fatalf("expected hello, got %q", es)
	}
}

func TestEncryptedString_ScanBytes(t *testing.T) {
	var es EncryptedString
	if err := es.Scan([]byte("world")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if es != "world" {
		t.Fatalf("expected world, got %q", es)
	}
}

func TestEncryptedString_ScanUnsupportedType(t *testing.T) {
	var es EncryptedString
	if err := es.Scan(42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if es != "" {
		t.Fatalf("expected empty for unsupported type, got %q", es)
	}
}

func TestEncryptedString_ValueEmpty(t *testing.T) {
	es := EncryptedString("")
	v, err := es.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestEncryptedString_ValueWithoutGlobalService(t *testing.T) {
	old := globalCryptoService
	globalCryptoService = nil
	defer func() { globalCryptoService = old }()

	es := EncryptedString("test")
	v, err := es.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "test" {
		t.Fatalf("expected test, got %q", v)
	}
}

func TestEncryptedString_RoundTripWithGlobalService(t *testing.T) {
	cs := testCryptoService(t)
	old := globalCryptoService
	globalCryptoService = cs
	defer func() { globalCryptoService = old }()

	es := EncryptedString("secret-value")
	v, err := es.Value()
	if err != nil {
		t.Fatalf("Value error: %v", err)
	}
	encrypted, ok := v.(string)
	if !ok {
		t.Fatal("Value should return string")
	}
	if !strings.HasPrefix(encrypted, storagePrefix) {
		t.Fatalf("expected encrypted prefix, got %q", encrypted)
	}

	// Scan back
	var es2 EncryptedString
	if err := es2.Scan(encrypted); err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if string(es2) != "secret-value" {
		t.Fatalf("round-trip failed: got %q", es2)
	}
}

func TestEncryptedString_String(t *testing.T) {
	es := EncryptedString("hello")
	if es.String() != "hello" {
		t.Fatalf("expected hello, got %q", es.String())
	}
}

// ---------------------------------------------------------------------------
// SetGlobalCryptoService
// ---------------------------------------------------------------------------

func TestSetGlobalCryptoService(t *testing.T) {
	old := globalCryptoService
	defer func() { globalCryptoService = old }()

	cs := testCryptoService(t)
	SetGlobalCryptoService(cs)
	if globalCryptoService != cs {
		t.Fatal("expected global crypto service to be set")
	}

	SetGlobalCryptoService(nil)
	if globalCryptoService != nil {
		t.Fatal("expected global crypto service to be nil")
	}
}
