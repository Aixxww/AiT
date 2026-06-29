package provider

import (
	"net/http"

	"github.com/Aixxww/AiT/mcp"
)

const (
	DefaultOpenAIBaseURL = "https://api.openai.com/v1"
	DefaultOpenAIModel   = "gpt-5.4"
)

func init() {
	mcp.RegisterProvider(mcp.ProviderOpenAI, func(opts ...mcp.ClientOption) mcp.AIClient {
		return NewOpenAIClientWithOptions(opts...)
	})
}

type OpenAIClient struct {
	*mcp.Client
}

func (c *OpenAIClient) BaseClient() *mcp.Client { return c.Client }

// NewOpenAIClient creates OpenAI client (backward compatible)
func NewOpenAIClient() mcp.AIClient {
	return NewOpenAIClientWithOptions()
}

// NewOpenAIClientWithOptions creates OpenAI client (supports options pattern)
func NewOpenAIClientWithOptions(opts ...mcp.ClientOption) mcp.AIClient {
	openaiOpts := []mcp.ClientOption{
		mcp.WithProvider(mcp.ProviderOpenAI),
		mcp.WithModel(DefaultOpenAIModel),
		mcp.WithBaseURL(DefaultOpenAIBaseURL),
	}

	allOpts := append(openaiOpts, opts...)
	baseClient := mcp.NewClient(allOpts...).(*mcp.Client)

	openaiClient := &OpenAIClient{
		Client: baseClient,
	}

	baseClient.Hooks = openaiClient
	return openaiClient
}

func (c *OpenAIClient) SetAPIKey(apiKey, customURL, customModel string) {
	c.Client.DefaultSetAPIKey(apiKey, customURL, customModel)
}

// OpenAI uses standard Bearer auth
func (c *OpenAIClient) SetAuthHeader(reqHeaders http.Header) {
	c.Client.SetAuthHeader(reqHeaders)
}
