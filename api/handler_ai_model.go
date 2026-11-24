package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/security"
	"github.com/Aixxww/AiT/store"
	"github.com/Aixxww/AiT/wallet"

	"github.com/gin-gonic/gin"
)

type ModelConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	Enabled      bool   `json:"enabled"`
	APIKey       string `json:"apiKey,omitempty"`
	CustomAPIURL string `json:"customApiUrl,omitempty"`
}

// SafeModelConfig Safe model configuration structure (does not contain sensitive information)
type SafeModelConfig struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Enabled         bool   `json:"enabled"`
	HasAPIKey       bool   `json:"has_api_key"`
	CustomAPIURL    string `json:"customApiUrl"`    // Custom API URL (usually not sensitive)
	CustomModelName string `json:"customModelName"` // Custom model name (not sensitive)
	WalletAddress   string `json:"walletAddress,omitempty"`
	BalanceUSDC     string `json:"balanceUsdc,omitempty"`
}

type UpdateModelConfigRequest struct {
	Models map[string]struct {
		Enabled         bool   `json:"enabled"`
		APIKey          string `json:"api_key"`
		CustomAPIURL    string `json:"custom_api_url"`
		CustomModelName string `json:"custom_model_name"`
		// Name and Provider are used by custom model entries (#75):
		// provider "custom" with a new id (or "custom") creates an entry,
		// an existing custom id updates it.
		Name     string `json:"name,omitempty"`
		Provider string `json:"provider,omitempty"`
	} `json:"models"`
}

// isCustomModelUpdate reports whether an update targets a custom entry.
func isCustomModelUpdate(modelID, provider string) bool {
	return strings.EqualFold(provider, store.ProviderCustom) || store.IsCustomModelID(modelID)
}

// handleGetModelConfigs Get AI model configurations
func (s *Server) handleGetModelConfigs(c *gin.Context) {
	userID := c.GetString("user_id")
	logger.Infof("🔍 Querying AI model configs for user %s", userID)
	models, err := s.store.AIModel().List(userID)
	if err != nil {
		logger.Infof("❌ Failed to get AI model configs: %v", err)
		SafeInternalError(c, "Failed to get AI model configs", err)
		return
	}

	// If no models in database, return default models
	if len(models) == 0 {
		logger.Infof("⚠️ No AI models in database, returning defaults")
		defaultModels := []SafeModelConfig{
			{ID: "deepseek", Name: "DeepSeek AI", Provider: "deepseek", Enabled: false, HasAPIKey: false},
			{ID: "qwen", Name: "Qwen AI", Provider: "qwen", Enabled: false, HasAPIKey: false},
			{ID: "openai", Name: "OpenAI", Provider: "openai", Enabled: false, HasAPIKey: false},
			{ID: "claude", Name: "Claude AI", Provider: "claude", Enabled: false, HasAPIKey: false},
			{ID: "gemini", Name: "Gemini AI", Provider: "gemini", Enabled: false, HasAPIKey: false},
			{ID: "grok", Name: "Grok AI", Provider: "grok", Enabled: false, HasAPIKey: false},
			{ID: "kimi", Name: "Kimi AI", Provider: "kimi", Enabled: false, HasAPIKey: false},
			{ID: "minimax", Name: "MiniMax AI", Provider: "minimax", Enabled: false, HasAPIKey: false},
			{ID: "mimo", Name: "Xiaomi MiMo", Provider: "mimo", Enabled: false, HasAPIKey: false},
		}
		c.JSON(http.StatusOK, defaultModels)
		return
	}

	logger.Infof("✅ Found %d AI model configs", len(models))

	// Convert to safe response structure, remove sensitive information
	safeModels := make([]SafeModelConfig, len(models))
	for i, model := range models {
		safeModel := SafeModelConfig{
			ID:              model.ID,
			Name:            model.Name,
			Provider:        model.Provider,
			Enabled:         model.Enabled,
			HasAPIKey:       model.APIKey != "",
			CustomAPIURL:    model.CustomAPIURL,
			CustomModelName: model.CustomModelName,
		}

		if model.Provider == "claw402" {
			if privateKey := strings.TrimSpace(model.APIKey.String()); privateKey != "" {
				if walletAddress, addrErr := walletAddressFromPrivateKey(privateKey); addrErr == nil {
					safeModel.WalletAddress = walletAddress
					safeModel.BalanceUSDC = wallet.QueryUSDCBalanceStr(walletAddress)
				} else {
					logger.Warnf("⚠️ Failed to derive claw402 wallet address for model %s: %v", model.ID, addrErr)
				}
			}
		}

		safeModels[i] = safeModel
	}

	c.JSON(http.StatusOK, safeModels)
}

// handleUpdateModelConfigs Update AI model configurations (supports both encrypted and plain text based on config)
func (s *Server) handleUpdateModelConfigs(c *gin.Context) {
	userID := c.GetString("user_id")

	var req UpdateModelConfigRequest
	if err := s.decryptOrParseJSON(c, &req); err != nil {
		logger.Infof("❌ Failed to parse model config (UserID: %s): %v", userID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Update each model's configuration and track traders that need reload
	tradersToReload := make(map[string]bool)
	for modelID, modelData := range req.Models {
		// SSRF protection: validate custom_api_url before storing
		if modelData.CustomAPIURL != "" {
			cleanURL := strings.TrimSuffix(modelData.CustomAPIURL, "#")
			if err := security.ValidateURL(cleanURL); err != nil {
				logger.Warnf("Invalid custom_api_url for model %s: %v", modelID, err)
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid custom_api_url for model %s: URL must be a valid HTTPS endpoint", modelID)})
				return
			}
		}

		// Find traders using this AI model BEFORE updating
		traders, _ := s.store.Trader().ListByAIModelID(userID, modelID)
		for _, t := range traders {
			tradersToReload[t.ID] = true
		}

		if isCustomModelUpdate(modelID, modelData.Provider) {
			if _, err := s.store.AIModel().UpsertCustom(userID, modelID, modelData.Name, modelData.Enabled, modelData.APIKey, modelData.CustomAPIURL, modelData.CustomModelName); err != nil {
				if errors.Is(err, store.ErrCustomModelInvalid) {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				SafeInternalError(c, fmt.Sprintf("Update custom model %s", modelID), err)
				return
			}
			continue
		}

		err := s.store.AIModel().Update(userID, modelID, modelData.Enabled, modelData.APIKey, modelData.CustomAPIURL, modelData.CustomModelName)
		if err != nil {
			SafeInternalError(c, fmt.Sprintf("Update model %s", modelID), err)
			return
		}
	}

	// Remove affected traders from memory BEFORE reloading to pick up new config
	for traderID := range tradersToReload {
		logger.Infof("🔄 Removing trader %s from memory to reload with new AI model config", traderID)
		s.traderManager.RemoveTrader(traderID)
	}

	// Reload all traders for this user to make new config take effect immediately
	if reloadErr := s.traderManager.LoadUserTradersFromStore(s.store, userID); reloadErr != nil {
		logger.Infof("⚠️ Failed to reload user traders into memory: %v", reloadErr)
		// Don't return error here since model config was successfully updated to database
	}

	logger.Infof("✓ AI model config updated: %+v", req.Models)
	c.JSON(http.StatusOK, gin.H{"message": "Model configuration updated"})
}

// handleGetSupportedModels Get list of AI models supported by the system
func (s *Server) handleGetSupportedModels(c *gin.Context) {
	// Return static list of supported AI models with default versions
	supportedModels := []map[string]interface{}{
		{"id": "deepseek", "name": "DeepSeek", "provider": "deepseek", "defaultModel": "deepseek-chat"},
		{"id": "qwen", "name": "Qwen", "provider": "qwen", "defaultModel": "qwen3-max"},
		{"id": "openai", "name": "OpenAI", "provider": "openai", "defaultModel": "gpt-5.1"},
		{"id": "claude", "name": "Claude", "provider": "claude", "defaultModel": "claude-opus-4-6"},
		{"id": "gemini", "name": "Google Gemini", "provider": "gemini", "defaultModel": "gemini-3.1-pro"},
		{"id": "grok", "name": "Grok (xAI)", "provider": "grok", "defaultModel": "grok-3-latest"},
		{"id": "kimi", "name": "Kimi (Moonshot)", "provider": "kimi", "defaultModel": "moonshot-v1-auto"},
		{"id": "minimax", "name": "MiniMax", "provider": "minimax", "defaultModel": "MiniMax-M2.7"},
		{"id": "mimo", "name": "Xiaomi MiMo", "provider": "mimo", "defaultModel": "mimo-v2.5-pro"},
		{"id": "claw402", "name": "Claw402 (Base USDC)", "provider": "claw402", "defaultModel": "deepseek-v4-flash"},
		// #75: user-defined OpenAI-compatible endpoint (base URL + model name required).
		{"id": "custom", "name": "Other (OpenAI-compatible)", "provider": "custom", "defaultModel": ""},
	}

	c.JSON(http.StatusOK, supportedModels)
}

// handleDeleteModelConfig deletes a custom model entry (#75). Built-in
// provider entries are disabled, not deleted. Traders that referenced the
// deleted entry are unloaded; on reload they fail with a "model does not
// exist" error instead of falling back to another model.
func (s *Server) handleDeleteModelConfig(c *gin.Context) {
	userID := c.GetString("user_id")
	modelID := c.Param("id")
	model, err := s.store.AIModel().Get(userID, modelID)
	if err != nil || model == nil || model.UserID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("AI model %s not found", modelID)})
		return
	}
	if model.Provider != store.ProviderCustom {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("AI model %s is a built-in %s entry; disable it instead of deleting it", modelID, model.Provider)})
		return
	}
	traders, _ := s.store.Trader().ListByAIModelID(userID, modelID)
	if err := s.store.AIModel().Delete(userID, modelID); err != nil {
		SafeInternalError(c, fmt.Sprintf("Delete model %s", modelID), err)
		return
	}
	for _, t := range traders {
		logger.Errorf("❌ Trader %s (%s) referenced deleted AI model %s; it will not start until another model is selected", t.Name, t.ID, modelID)
		if s.traderManager != nil {
			s.traderManager.RemoveTrader(t.ID)
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "Model deleted", "affected_traders": len(traders)})
}
