package provider

import (
	"net/http"
	"time"

	"github.com/Aixxww/AiT/mcp"
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

func (c *MiMoClient) SetAPIKey(apiKey, customURL, customModel string) {
	c.Client.DefaultSetAPIKey(apiKey, customURL, customModel)
}

func (c *MiMoClient) SetAuthHeader(reqHeaders http.Header) {
	c.Client.SetAuthHeader(reqHeaders)
}
