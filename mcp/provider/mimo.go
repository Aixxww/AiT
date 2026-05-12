package provider

import (
	"net/http"
	"time"

	"nofx/mcp"
)

func init() {
	mcp.RegisterProvider(mcp.ProviderMiMo, func(opts ...mcp.ClientOption) mcp.AIClient {
		return NewMiMoClientWithOptions(opts...)
	})
}

type MiMoClient struct {
	*mcp.Client
}

func (c *MiMoClient) BaseClient() *mcp.Client { return c.Client }

func NewMiMoClient() mcp.AIClient {
	return NewMiMoClientWithOptions()
}

func NewMiMoClientWithOptions(opts ...mcp.ClientOption) mcp.AIClient {
	mimoOpts := []mcp.ClientOption{
		mcp.WithProvider(mcp.ProviderMiMo),
		mcp.WithModel(mcp.DefaultMiMoModel),
		mcp.WithBaseURL(mcp.DefaultMiMoBaseURL),
		mcp.WithTimeout(8 * time.Minute), // MiMo at Amsterdam endpoint has high latency
	}

	allOpts := append(mimoOpts, opts...)
	baseClient := mcp.NewClient(allOpts...).(*mcp.Client)

	mimoClient := &MiMoClient{
		Client: baseClient,
	}

	baseClient.Hooks = mimoClient
	return mimoClient
}

func (mimoClient *MiMoClient) SetAPIKey(apiKey string, customURL string, customModel string) {
	mimoClient.APIKey = apiKey

	if len(apiKey) > 8 {
		mimoClient.Log.Infof("🔧 [MCP] MiMo API Key: %s...%s", apiKey[:4], apiKey[len(apiKey)-4:])
	}
	if customURL != "" {
		mimoClient.BaseURL = customURL
		mimoClient.Log.Infof("🔧 [MCP] MiMo using custom BaseURL: %s", customURL)
	} else {
		mimoClient.Log.Infof("🔧 [MCP] MiMo using default BaseURL: %s", mimoClient.BaseURL)
	}
	if customModel != "" {
		mimoClient.Model = customModel
		mimoClient.Log.Infof("🔧 [MCP] MiMo using custom Model: %s", customModel)
	} else {
		mimoClient.Log.Infof("🔧 [MCP] MiMo using default Model: %s", mimoClient.Model)
	}
}

func (mimoClient *MiMoClient) SetAuthHeader(reqHeaders http.Header) {
	mimoClient.Client.SetAuthHeader(reqHeaders)
}
