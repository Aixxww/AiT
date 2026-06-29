package provider

import (
	"net/http"

	"github.com/Aixxww/AiT/mcp"
)

func init() {
	mcp.RegisterProvider(mcp.ProviderDeepSeek, func(opts ...mcp.ClientOption) mcp.AIClient {
		return NewDeepSeekClientWithOptions(opts...)
	})
}

type DeepSeekClient struct {
	*mcp.Client
}

func (c *DeepSeekClient) BaseClient() *mcp.Client { return c.Client }

// NewDeepSeekClient creates DeepSeek client (backward compatible)
//
// Deprecated: Recommend using NewDeepSeekClientWithOptions for better flexibility
func NewDeepSeekClient() mcp.AIClient {
	return NewDeepSeekClientWithOptions()
}

// NewDeepSeekClientWithOptions creates DeepSeek client (supports options pattern)
func NewDeepSeekClientWithOptions(opts ...mcp.ClientOption) mcp.AIClient {
	deepseekOpts := []mcp.ClientOption{
		mcp.WithProvider(mcp.ProviderDeepSeek),
		mcp.WithModel(mcp.DefaultDeepSeekModel),
		mcp.WithBaseURL(mcp.DefaultDeepSeekBaseURL),
	}

	allOpts := append(deepseekOpts, opts...)
	baseClient := mcp.NewClient(allOpts...).(*mcp.Client)

	dsClient := &DeepSeekClient{
		Client: baseClient,
	}

	baseClient.Hooks = dsClient
	return dsClient
}

func (dsClient *DeepSeekClient) SetAPIKey(apiKey, customURL, customModel string) {
	dsClient.Client.DefaultSetAPIKey(apiKey, customURL, customModel)
}

func (dsClient *DeepSeekClient) SetAuthHeader(reqHeaders http.Header) {
	dsClient.Client.SetAuthHeader(reqHeaders)
}
