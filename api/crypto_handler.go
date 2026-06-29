package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/Aixxww/AiT/config"
	"github.com/Aixxww/AiT/crypto"

	"github.com/gin-gonic/gin"
)

// CryptoHandler Encryption API handler
type CryptoHandler struct {
	cryptoService *crypto.CryptoService
}

// NewCryptoHandler Creates encryption handler
func NewCryptoHandler(cryptoService *crypto.CryptoService) *CryptoHandler {
	return &CryptoHandler{
		cryptoService: cryptoService,
	}
}

// ==================== Crypto Config Endpoint ====================

// HandleGetCryptoConfig Get crypto configuration
func (h *CryptoHandler) HandleGetCryptoConfig(c *gin.Context) {
	cfg := config.Get()
	c.JSON(http.StatusOK, gin.H{
		"transport_encryption": cfg.TransportEncryption,
	})
}

// ==================== Public Key Endpoint ====================

// HandleGetPublicKey Get server public key
func (h *CryptoHandler) HandleGetPublicKey(c *gin.Context) {
	cfg := config.Get()
	if !cfg.TransportEncryption {
		c.JSON(http.StatusOK, gin.H{
			"public_key":           "",
			"algorithm":            "",
			"transport_encryption": false,
		})
		return
	}

	publicKey := h.cryptoService.GetPublicKeyPEM()
	c.JSON(http.StatusOK, gin.H{
		"public_key":           publicKey,
		"algorithm":            "RSA-OAEP-2048",
		"transport_encryption": true,
	})
}

// ==================== Encrypted Data Decryption Endpoint ====================

// HandleDecryptSensitiveData Decrypt encrypted data sent from client
func (h *CryptoHandler) HandleDecryptSensitiveData(c *gin.Context) {
	var payload crypto.EncryptedPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Decrypt
	decrypted, err := h.cryptoService.DecryptSensitiveData(&payload)
	if err != nil {
		log.Printf("❌ Decryption failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Decryption failed"})
		return
	}

	c.JSON(http.StatusOK, map[string]string{
		"plaintext": decrypted,
	})
}

// ==================== Audit Log Query Endpoint ====================

// Audit log functionality removed, not needed in current simplified implementation

// ==================== Shared Decrypt-or-Parse Helper ====================

// decryptOrParseJSON reads the raw request body and, depending on whether
// transport encryption is enabled, either parses plain JSON or decrypts an
// encrypted payload first. The result is unmarshalled into dst.
//
// This consolidates the identical decrypt-or-parse blocks previously
// duplicated in handleUpdateExchangeConfigs, handleCreateExchange,
// and handleUpdateModelConfigs.
func (s *Server) decryptOrParseJSON(c *gin.Context, dst interface{}) error {
	bodyBytes, err := c.GetRawData()
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	cfg := config.Get()
	if !cfg.TransportEncryption {
		if err := json.Unmarshal(bodyBytes, dst); err != nil {
			return fmt.Errorf("invalid request format: %w", err)
		}
		return nil
	}

	var encryptedPayload crypto.EncryptedPayload
	if err := json.Unmarshal(bodyBytes, &encryptedPayload); err != nil {
		return fmt.Errorf("invalid request format, encrypted transmission required: %w", err)
	}

	if encryptedPayload.WrappedKey == "" {
		return fmt.Errorf("encrypted transmission required: missing wrapped key")
	}

	decrypted, err := s.cryptoHandler.cryptoService.DecryptSensitiveData(&encryptedPayload)
	if err != nil {
		return fmt.Errorf("failed to decrypt data: %w", err)
	}

	if err := json.Unmarshal([]byte(decrypted), dst); err != nil {
		return fmt.Errorf("failed to parse decrypted data: %w", err)
	}

	return nil
}

// ==================== Utility Functions ====================

// isValidPrivateKey Validate private key format
func isValidPrivateKey(key string) bool {
	// EVM private key: 64 hex characters (optional 0x prefix)
	if len(key) == 64 || (len(key) == 66 && key[:2] == "0x") {
		return true
	}
	// TODO: Add validation for other chains
	return false
}
