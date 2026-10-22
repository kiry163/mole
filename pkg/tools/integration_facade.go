package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kiry163/mole/pkg/config"
	integrationtools "github.com/kiry163/mole/pkg/tools/integration"
)

type (
	SendCallbackWithContext = integrationtools.SendCallbackWithContext
	ReactionCallback        = integrationtools.ReactionCallback
	MCPManager              = integrationtools.MCPManager
	MCPTool                 = integrationtools.MCPTool
	MessageTool             = integrationtools.MessageTool
	ReactionTool            = integrationtools.ReactionTool
	APIKeyPool              = integrationtools.APIKeyPool
	APIKeyIterator          = integrationtools.APIKeyIterator
	SearchProvider          = integrationtools.SearchProvider
	SearchResultItem        = integrationtools.SearchResultItem
	TavilySearchProvider    = integrationtools.TavilySearchProvider
	SogouSearchProvider     = integrationtools.SogouSearchProvider
	WebSearchTool           = integrationtools.WebSearchTool
	WebSearchToolOptions    = integrationtools.WebSearchToolOptions
	WebFetchTool            = integrationtools.WebFetchTool
)

func NewMCPTool(manager MCPManager, serverName string, tool *mcp.Tool) *MCPTool {
	return integrationtools.NewMCPTool(manager, serverName, tool)
}

func NewMessageTool() *MessageTool {
	return integrationtools.NewMessageTool()
}

func NewReactionTool() *ReactionTool {
	return integrationtools.NewReactionTool()
}

func WebSearchToolOptionsFromConfig(cfg *config.Config) WebSearchToolOptions {
	return integrationtools.WebSearchToolOptionsFromConfig(cfg)
}

func WebSearchProviderReady(opts WebSearchToolOptions, name string) bool {
	return integrationtools.WebSearchProviderReady(opts, name)
}

func ResolveWebSearchProviderName(opts WebSearchToolOptions, query string) (string, error) {
	return integrationtools.ResolveWebSearchProviderName(opts, query)
}

func NewWebSearchTool(opts WebSearchToolOptions) (*WebSearchTool, error) {
	return integrationtools.NewWebSearchTool(opts)
}

func NewWebFetchToolWithProxy(
	maxChars int,
	proxy string,
	format string,
	fetchLimitBytes int64,
	privateHostWhitelist []string,
) (*WebFetchTool, error) {
	return integrationtools.NewWebFetchToolWithProxy(maxChars, proxy, format, fetchLimitBytes, privateHostWhitelist)
}
