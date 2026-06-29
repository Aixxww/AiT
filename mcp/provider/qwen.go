package provider

import (
	"net/http"

	"github.com/Aixxww/AiT/mcp"
)

const (
	DefaultQwenBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	DefaultQwenModel   = "qwen3-max"
)

func init() {
	mcp.RegisterProvider(mcp.ProviderQwen, func(opts ...mcp.ClientOption) mcp.AIClient {
		return NewQwenClientWithOptions(opts...)
	})
}

type QwenClient struct {
	*mcp.Client
}

func (c *QwenClient) BaseClient() *mcp.Client { return c.Client }

// NewQwenClient creates Qwen client (backward compatible)
//
// Deprecated: Recommend using NewQwenClientWithOptions for better flexibility
func NewQwenClient() mcp.AIClient {
	return NewQwenClientWithOptions()
}

// NewQwenClientWithOptions creates Qwen client (supports options pattern)
func NewQwenClientWithOptions(opts ...mcp.ClientOption) mcp.AIClient {
	qwenOpts := []mcp.ClientOption{
		mcp.WithProvider(mcp.ProviderQwen),
		mcp.WithModel(DefaultQwenModel),
		mcp.WithBaseURL(DefaultQwenBaseURL),
	}

	allOpts := append(qwenOpts, opts...)
	baseClient := mcp.NewClient(allOpts...).(*mcp.Client)

	qwenClient := &QwenClient{
		Client: baseClient,
	}

	baseClient.Hooks = qwenClient
	return qwenClient
}

func (c *QwenClient) SetAPIKey(apiKey, customURL, customModel string) {
	c.Client.DefaultSetAPIKey(apiKey, customURL, customModel)
}

func (c *QwenClient) SetAuthHeader(reqHeaders http.Header) {
	c.Client.SetAuthHeader(reqHeaders)
}
