package integrationtools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/logger"
	"github.com/kiry163/mole/pkg/utils"
)

const (
	userAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	sogouUserAgent  = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1"
	userAgentHonest = "mole/%s (+https://github.com/kiry163/mole; AI assistant bot)"

	// HTTP client timeouts for web tool providers.
	searchTimeout = 10 * time.Second // Tavily, Sogou
	fetchTimeout  = 60 * time.Second // WebFetchTool

	defaultMaxChars = 50000
	maxRedirects    = 5
)

// Pre-compiled regexes for HTML text extraction
var (
	reScript     = regexp.MustCompile(`<script[\s\S]*?</script>`)
	reStyle      = regexp.MustCompile(`<style[\s\S]*?</style>`)
	reTags       = regexp.MustCompile(`<[^>]+>`)
	reWhitespace = regexp.MustCompile(`[^\S\n]+`)
	reBlankLines = regexp.MustCompile(`\n{3,}`)

	reSogouTitle = regexp.MustCompile(
		`<a\s+class="?resultLink"?\s+href="([^"]+)"[^>]*id="sogou_vr_\d+_\d+"[^>]*>\s*(.*?)\s*</a>`,
	)
	reSogouSnippet = regexp.MustCompile(`<div class="clamp\d*[^"]*">\s*(.*?)\s*</div>`)
	reSogouRealURL = regexp.MustCompile(`url=([^&]+)`)
)

type APIKeyPool struct {
	keys    []string
	current uint32
}

func NewAPIKeyPool(keys []string) *APIKeyPool {
	return &APIKeyPool{
		keys: keys,
	}
}

type APIKeyIterator struct {
	pool     *APIKeyPool
	startIdx uint32
	attempt  uint32
}

func (p *APIKeyPool) NewIterator() *APIKeyIterator {
	if len(p.keys) == 0 {
		return &APIKeyIterator{pool: p}
	}
	idx := atomic.AddUint32(&p.current, 1) - 1
	return &APIKeyIterator{
		pool:     p,
		startIdx: idx,
	}
}

func (it *APIKeyIterator) Next() (string, bool) {
	length := uint32(len(it.pool.keys))
	if length == 0 || it.attempt >= length {
		return "", false
	}
	key := it.pool.keys[(it.startIdx+it.attempt)%length]
	it.attempt++
	return key, true
}

type SearchProvider interface {
	Search(ctx context.Context, query string, count int, rangeCode string) (string, error)
}

type SearchResultItem struct {
	Title     string
	URL       string
	Snippet   string
	Published string
}

func extractSogouURL(href string) string {
	match := reSogouRealURL.FindStringSubmatch(href)
	if len(match) < 2 {
		return ""
	}
	decoded, err := url.QueryUnescape(match[1])
	if err != nil {
		return ""
	}
	return decoded
}

func applySogouRangeHint(query string, rangeCode string) string {
	switch rangeCode {
	case "d":
		return query + " 最近一天"
	case "w":
		return query + " 最近一周"
	case "m":
		return query + " 最近一个月"
	case "y":
		return query + " 最近一年"
	default:
		return query
	}
}

func normalizeSearchRange(raw string) (string, error) {
	rangeCode := strings.ToLower(strings.TrimSpace(raw))
	switch rangeCode {
	case "", "d", "w", "m", "y":
		return rangeCode, nil
	default:
		return "", fmt.Errorf("range must be one of: d, w, m, y")
	}
}

func mapTavilyTimeRange(rangeCode string) string {
	switch rangeCode {
	case "d":
		return "day"
	case "w":
		return "week"
	case "m":
		return "month"
	case "y":
		return "year"
	default:
		return ""
	}
}

type TavilySearchProvider struct {
	keyPool *APIKeyPool
	baseURL string
	proxy   string
	client  *http.Client
}

func (p *TavilySearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	if p.keyPool == nil || len(p.keyPool.keys) == 0 {
		return "", errors.New("no API key provided")
	}

	searchURL := p.baseURL
	if searchURL == "" {
		searchURL = "https://api.tavily.com/search"
	}

	var lastErr error
	iter := p.keyPool.NewIterator()

	for {
		apiKey, ok := iter.Next()
		if !ok {
			break
		}

		payload := map[string]any{
			"api_key":             apiKey,
			"query":               query,
			"search_depth":        "advanced",
			"include_answer":      false,
			"include_images":      false,
			"include_raw_content": false,
			"max_results":         count,
		}
		if timeRange := mapTavilyTimeRange(rangeCode); timeRange != "" {
			payload["time_range"] = timeRange
		}

		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("failed to marshal payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", searchURL, bytes.NewBuffer(bodyBytes))
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgent)

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("tavily api error (status %d): %s", resp.StatusCode, string(body))
			if resp.StatusCode == http.StatusTooManyRequests ||
				resp.StatusCode == http.StatusUnauthorized ||
				resp.StatusCode == http.StatusForbidden ||
				resp.StatusCode >= 500 {
				continue
			}
			return "", lastErr
		}

		var searchResp struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
			} `json:"results"`
		}

		if err := json.Unmarshal(body, &searchResp); err != nil {
			return "", fmt.Errorf("failed to parse response: %w", err)
		}

		results := searchResp.Results
		if len(results) == 0 {
			return fmt.Sprintf("No results for: %s", query), nil
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("Results for: %s (via Tavily)", query))
		for i, item := range results {
			if i >= count {
				break
			}
			lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.URL))
			if item.Content != "" {
				lines = append(lines, fmt.Sprintf("   %s", item.Content))
			}
		}

		return strings.Join(lines, "\n"), nil
	}

	return "", fmt.Errorf("all api keys failed, last error: %w", lastErr)
}

type SogouSearchProvider struct {
	proxy  string
	client *http.Client
}

func (p *SogouSearchProvider) Search(
	ctx context.Context,
	query string,
	count int,
	rangeCode string,
) (string, error) {
	const sogouWAPURL = "https://wap.sogou.com/web/searchList.jsp"

	results := make([]SearchResultItem, 0, count)
	seenURLs := make(map[string]bool)
	maxPages := min(3, (count+1)/2+1)

	for page := 1; page <= maxPages && len(results) < count; page++ {
		params := url.Values{}
		params.Set("keyword", applySogouRangeHint(query, rangeCode))
		params.Set("v", "5")
		params.Set("p", fmt.Sprintf("%d", page))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sogouWAPURL+"?"+params.Encode(), nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("User-Agent", sogouUserAgent)

		resp, err := p.client.Do(req)
		if err != nil {
			return "", fmt.Errorf("request failed: %w", err)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("Sogou returned status %d", resp.StatusCode)
		}

		html := string(body)
		if len(html) < 200 {
			break
		}

		matches := reSogouTitle.FindAllStringSubmatch(html, -1)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}

			title := stripTags(match[2])
			link := extractSogouURL(match[1])
			if title == "" || link == "" || seenURLs[link] {
				continue
			}
			seenURLs[link] = true

			start := strings.Index(html, match[0])
			snippet := ""
			if start >= 0 {
				after := html[start+len(match[0]):]
				if len(after) > 2000 {
					after = after[:2000]
				}
				if snippetMatch := reSogouSnippet.FindStringSubmatch(after); len(snippetMatch) > 1 {
					snippet = stripTags(snippetMatch[1])
				}
			}

			results = append(results, SearchResultItem{
				Title:   title,
				URL:     link,
				Snippet: snippet,
			})
			if len(results) >= count {
				break
			}
		}
	}

	if len(results) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	lines := []string{fmt.Sprintf("Results for: %s (via Sogou)", query)}
	for i, item := range results {
		lines = append(lines, fmt.Sprintf("%d. %s\n   %s", i+1, item.Title, item.URL))
		if item.Snippet != "" {
			lines = append(lines, fmt.Sprintf("   %s", item.Snippet))
		}
	}
	return strings.Join(lines, "\n"), nil
}

func stripTags(content string) string {
	return reTags.ReplaceAllString(content, "")
}

type WebSearchTool struct {
	provider         SearchProvider
	maxResults       int
	providerResolver func(query string) (SearchProvider, int)
}

type WebSearchToolOptions struct {
	Provider         string
	TavilyAPIKeys    []string
	TavilyBaseURL    string
	TavilyMaxResults int
	TavilyEnabled    bool
	SogouMaxResults  int
	SogouEnabled     bool
	Proxy            string
}

func WebSearchToolOptionsFromConfig(cfg *config.Config) WebSearchToolOptions {
	return WebSearchToolOptions{
		Provider:         cfg.Tools.Web.Provider,
		TavilyAPIKeys:    cfg.Tools.Web.Tavily.APIKeys.Values(),
		TavilyBaseURL:    cfg.Tools.Web.Tavily.BaseURL,
		TavilyMaxResults: cfg.Tools.Web.Tavily.MaxResults,
		TavilyEnabled:    cfg.Tools.Web.Tavily.Enabled,
		SogouMaxResults:  cfg.Tools.Web.Sogou.MaxResults,
		SogouEnabled:     cfg.Tools.Web.Sogou.Enabled,
		Proxy:            cfg.Tools.Web.Proxy,
	}
}

func WebSearchProviderReady(opts WebSearchToolOptions, name string) bool {
	return opts.providerReady(name)
}

func ResolveWebSearchProviderName(opts WebSearchToolOptions, query string) (string, error) {
	return opts.resolveProviderName(query)
}

var (
	knownWebSearchProviders = []string{
		"sogou",
		"tavily",
	}
	autoPrimaryWebSearchProviders  = []string{"tavily"}
	autoFallbackWebSearchProviders = []string{"sogou"}
)

func isKnownWebSearchProvider(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, known := range knownWebSearchProviders {
		if name == known {
			return true
		}
	}
	return false
}

func (opts WebSearchToolOptions) providerReady(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sogou":
		return opts.SogouEnabled
	case "tavily":
		return opts.TavilyEnabled && len(opts.TavilyAPIKeys) > 0
	default:
		return false
	}
}

func (opts WebSearchToolOptions) normalizedProviderName() string {
	providerName := strings.ToLower(strings.TrimSpace(opts.Provider))
	if providerName != "" && providerName != "auto" && !isKnownWebSearchProvider(providerName) {
		// Tolerate stale or manually edited config values at runtime by
		// treating them as "auto" and falling back to the next ready provider.
		return "auto"
	}
	return providerName
}

// resolveProviderName picks the provider to use. An explicitly configured
// provider wins when it is ready; otherwise (including a stale/unready one) the
// "auto" order applies: tavily -> sogou. When nothing is ready it returns a
// readable error instead of an empty provider name.
func (opts WebSearchToolOptions) resolveProviderName(query string) (string, error) {
	providerName := opts.normalizedProviderName()
	if providerName != "" && providerName != "auto" && opts.providerReady(providerName) {
		return providerName, nil
	}

	for _, name := range autoPrimaryWebSearchProviders {
		if opts.providerReady(name) {
			return name, nil
		}
	}
	for _, name := range autoFallbackWebSearchProviders {
		if opts.providerReady(name) {
			return name, nil
		}
	}

	return "", fmt.Errorf("no web search provider available: configure tools.web.tavily.api_keys, or enable tools.web.sogou")
}

func (opts WebSearchToolOptions) providerByName(name string) (SearchProvider, int, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "auto":
		return nil, 0, nil
	case "sogou":
		if !opts.providerReady("sogou") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for Sogou: %w", err)
		}
		maxResults := 10
		if opts.SogouMaxResults > 0 {
			maxResults = min(opts.SogouMaxResults, 10)
		}
		return &SogouSearchProvider{
			proxy:  opts.Proxy,
			client: client,
		}, maxResults, nil
	case "tavily":
		if !opts.providerReady("tavily") {
			return nil, 0, nil
		}
		client, err := utils.CreateHTTPClient(opts.Proxy, searchTimeout)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create HTTP client for Tavily: %w", err)
		}
		maxResults := 10
		if opts.TavilyMaxResults > 0 {
			maxResults = min(opts.TavilyMaxResults, 10)
		}
		return &TavilySearchProvider{
			keyPool: NewAPIKeyPool(opts.TavilyAPIKeys),
			baseURL: opts.TavilyBaseURL,
			proxy:   opts.Proxy,
			client:  client,
		}, maxResults, nil
	default:
		return nil, 0, nil
	}
}

func (opts WebSearchToolOptions) buildProviderResolver() (func(query string) (SearchProvider, int), error) {
	providersByName := make(map[string]SearchProvider, len(knownWebSearchProviders))
	maxResultsByName := make(map[string]int, len(knownWebSearchProviders))

	for _, name := range knownWebSearchProviders {
		if !opts.providerReady(name) {
			continue
		}
		provider, maxResults, err := opts.providerByName(name)
		if err != nil {
			return nil, err
		}
		if provider == nil {
			continue
		}
		providersByName[name] = provider
		maxResultsByName[name] = maxResults
	}

	return func(query string) (SearchProvider, int) {
		name, err := opts.resolveProviderName(query)
		if err != nil {
			return nil, 0
		}
		provider, ok := providersByName[name]
		if !ok {
			return nil, 0
		}
		return provider, maxResultsByName[name]
	}, nil
}

func NewWebSearchTool(opts WebSearchToolOptions) (*WebSearchTool, error) {
	resolver, err := opts.buildProviderResolver()
	if err != nil {
		return nil, err
	}
	provider, maxResults := resolver("")
	if provider == nil {
		return nil, nil
	}

	return &WebSearchTool{
		provider:         provider,
		maxResults:       maxResults,
		providerResolver: resolver,
	}, nil
}

func (t *WebSearchTool) Name() string {
	return "web_search"
}

func (t *WebSearchTool) Description() string {
	return "Search the web for current information. Supports query, count, and an optional temporal range filter. Returns titles, URLs, and snippets from search results."
}

func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"count": map[string]any{
				"type":        "integer",
				"description": "Number of results (default: 10, max: 10)",
				"minimum":     1.0,
				"maximum":     10.0,
			},
			"range": map[string]any{
				"type":        "string",
				"description": "Optional time filter: d (day), w (week), m (month), y (year)",
				"enum":        []string{"d", "w", "m", "y"},
			},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return ErrorResult("query is required")
	}
	query = strings.TrimSpace(query)

	provider := t.provider
	maxResults := t.maxResults
	if t.providerResolver != nil {
		provider, maxResults = t.providerResolver(query)
	}
	if provider == nil {
		return ErrorResult("search provider is not configured")
	}

	count64, err := getInt64Arg(args, "count", int64(maxResults))
	if err != nil {
		return ErrorResult(err.Error())
	}
	count := maxResults
	if count64 > 0 && count64 <= 10 {
		count = min(int(count64), maxResults)
	}

	rangeCode, err := normalizeSearchRange("")
	if err != nil {
		return ErrorResult(err.Error())
	}
	if rawRange, exists := args["range"]; exists {
		rangeStr, ok := rawRange.(string)
		if !ok {
			return ErrorResult("range must be a string")
		}
		rangeCode, err = normalizeSearchRange(rangeStr)
		if err != nil {
			return ErrorResult(err.Error())
		}
	}

	result, err := provider.Search(ctx, query, count, rangeCode)
	if err != nil {
		return ErrorResult(fmt.Sprintf("search failed: %v", err))
	}

	return &ToolResult{
		ForLLM:  result,
		ForUser: result,
	}
}

type WebFetchTool struct {
	maxChars        int
	proxy           string
	client          *http.Client
	format          string
	fetchLimitBytes int64
	whitelist       *utils.PrivateHostWhitelist
}

func NewWebFetchTool(maxChars int, format string, fetchLimitBytes int64) (*WebFetchTool, error) {
	// createHTTPClient cannot fail with an empty proxy string.
	return NewWebFetchToolWithConfig(maxChars, "", format, fetchLimitBytes, nil)
}

// allowPrivateWebFetchHosts controls whether loopback/private hosts are allowed.
// This is false in normal runtime to reduce SSRF exposure, and tests can override it temporarily.
var allowPrivateWebFetchHosts atomic.Bool

func NewWebFetchToolWithProxy(
	maxChars int,
	proxy string,
	format string,
	fetchLimitBytes int64,
	privateHostWhitelist []string,
) (*WebFetchTool, error) {
	return NewWebFetchToolWithConfig(maxChars, proxy, format, fetchLimitBytes, privateHostWhitelist)
}

func NewWebFetchToolWithConfig(
	maxChars int,
	proxy string,
	format string,
	fetchLimitBytes int64,
	privateHostWhitelist []string,
) (*WebFetchTool, error) {
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}
	whitelist, err := utils.NewPrivateHostWhitelist(privateHostWhitelist)
	if err != nil {
		return nil, fmt.Errorf("failed to parse web fetch private host whitelist: %w", err)
	}
	client, err := utils.CreateSafeHTTPClient(utils.SafeHTTPClientOptions{
		ProxyURL:             proxy,
		Timeout:              fetchTimeout,
		PrivateHostWhitelist: privateHostWhitelist,
		AllowPrivateHosts: func() bool {
			return allowPrivateWebFetchHosts.Load()
		},
		MaxRedirects: maxRedirects,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client for web fetch: %w", err)
	}
	if fetchLimitBytes <= 0 {
		fetchLimitBytes = 10 * 1024 * 1024 // Security Fallback
	}
	return &WebFetchTool{
		maxChars:        maxChars,
		proxy:           proxy,
		client:          client,
		format:          format,
		fetchLimitBytes: fetchLimitBytes,
		whitelist:       whitelist,
	}, nil
}

func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

func (t *WebFetchTool) Description() string {
	return "Fetch a URL and extract readable content (HTML to text). Use this to get weather info, news, articles, or any web content."
}

func (t *WebFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "URL to fetch",
			},
			"maxChars": map[string]any{
				"type":        "integer",
				"description": "Maximum characters to extract",
				"minimum":     100.0,
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	urlStr, ok := args["url"].(string)
	if !ok {
		return ErrorResult("url is required")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid URL: %v", err))
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return ErrorResult("only http/https URLs are allowed")
	}

	if parsedURL.Host == "" {
		return ErrorResult("missing domain in URL")
	}

	// Lightweight pre-flight: block obvious localhost/literal-IP without DNS resolution.
	// The real SSRF guard is newSafeDialContext at connect time.
	hostname := parsedURL.Hostname()
	if utils.IsObviousPrivateHost(hostname, t.whitelist, func() bool {
		return allowPrivateWebFetchHosts.Load()
	}) {
		return ErrorResult("fetching private or local network hosts is not allowed")
	}

	maxChars := t.maxChars
	if mc, ok := args["maxChars"].(float64); ok {
		if int(mc) > 100 {
			maxChars = int(mc)
		}
	}

	doFetch := func(ua string) (*http.Response, []byte, error) {
		req, reqErr := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if reqErr != nil {
			return nil, nil, fmt.Errorf("failed to create request: %w", reqErr)
		}
		utils.AllowConfiguredProxyFirstHop(req, t.client.Transport)
		req.Header.Set("User-Agent", ua)
		resp, doErr := t.client.Do(req)
		if doErr != nil {
			return nil, nil, fmt.Errorf("request failed: %w", doErr)
		}
		resp.Body = http.MaxBytesReader(nil, resp.Body, t.fetchLimitBytes)

		b, readErr := io.ReadAll(resp.Body)
		return resp, b, readErr
	}

	resp, body, err := doFetch(userAgent)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return ErrorResult(
				fmt.Sprintf(
					"failed to read response: size exceeded %d bytes limit",
					t.fetchLimitBytes,
				),
			)
		}
		return ErrorResult(err.Error())
	}

	// Cloudflare (and similar WAFs) signal bot challenges with 403 + cf-mitigated: challenge.
	// Retry once with an honest User-Agent that identifies mole, which some
	// operators explicitly allow-list for AI assistants.
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("Cf-Mitigated") == "challenge" {
		logger.DebugCF("tool", "Cloudflare challenge detected, retrying with honest User-Agent",
			map[string]any{"url": urlStr})
		honestUA := fmt.Sprintf(userAgentHonest, config.Version)
		resp2, body2, err2 := doFetch(honestUA)
		if resp2 != nil && resp2.Body != nil {
			defer resp2.Body.Close()
		}

		if err2 == nil {
			resp, body = resp2, body2
		} else {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err2, &maxBytesErr) {
				return ErrorResult(
					fmt.Sprintf("failed to read response: size exceeded %d bytes limit", t.fetchLimitBytes),
				)
			}
			return ErrorResult(err2.Error())
		}
	}

	bodyStr := string(body)
	contentType := resp.Header.Get("Content-Type")

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		// The most common error here is "mime: no media type" if the header is empty.
		logger.WarnCF("tool", "Failed to parse Content-Type", map[string]any{
			"raw_header": contentType,
			"error":      err.Error(),
		})

		// security fallback
		mediaType = "application/octet-stream"
	}

	charset, hasCharset := params["charset"]
	if hasCharset {
		// If the charset is not utf-8, we might have to convert the bodyStr
		// before passing it to the HTML/Markdown parser
		if strings.ToLower(charset) != "utf-8" {
			logger.WarnCF(
				"tool",
				"Note: the content is not in UTF-8",
				map[string]any{"charset": charset},
			)
		}
	}

	var text, extractor string

	switch {
	case mediaType == "application/json":
		var jsonData any
		if err := json.Unmarshal(body, &jsonData); err != nil {
			text = bodyStr
			extractor = "raw"
			break
		}

		formatted, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			text = bodyStr
			extractor = "raw"
			break
		}

		text = string(formatted)
		extractor = "json"

	case mediaType == "text/html" || looksLikeHTML(bodyStr):
		switch strings.ToLower(t.format) {
		case "markdown":
			var err error
			text, err = utils.HtmlToMarkdown(bodyStr)
			if err != nil {
				return ErrorResult(fmt.Sprintf("failed to HTML to markdown: %v", err))
			}
			extractor = "markdown"

		default:
			text = t.extractText(bodyStr)
			extractor = "text"
		}

	default:
		text = bodyStr
		extractor = "raw"
	}

	truncated := len(text) > maxChars
	if truncated {
		text = text[:maxChars] + "\n[Content truncated due to size limit]"
	}

	result := map[string]any{
		"url":       urlStr,
		"status":    resp.StatusCode,
		"extractor": extractor,
		"truncated": truncated,
		"length":    len(text),
		"text":      text,
	}

	resultJSON, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal result: %v", marshalErr))
	}

	return &ToolResult{
		ForLLM: string(resultJSON),
		ForUser: fmt.Sprintf(
			"Fetched %d bytes from %s (extractor: %s, truncated: %v)",
			len(text),
			urlStr,
			extractor,
			truncated,
		),
	}
}

func looksLikeHTML(body string) bool {
	if body == "" {
		return false
	}

	lower := strings.ToLower(body)

	return strings.HasPrefix(body, "<!doctype") ||
		strings.HasPrefix(lower, "<html")
}

func (t *WebFetchTool) extractText(htmlContent string) string {
	result := reScript.ReplaceAllLiteralString(htmlContent, "")
	result = reStyle.ReplaceAllLiteralString(result, "")
	result = reTags.ReplaceAllLiteralString(result, "")

	result = strings.TrimSpace(result)

	result = reWhitespace.ReplaceAllString(result, " ")
	result = reBlankLines.ReplaceAllString(result, "\n\n")

	lines := strings.Split(result, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}

func newSafeDialContext(
	dialer *net.Dialer,
	whitelist *utils.PrivateHostWhitelist,
) func(context.Context, string, string) (net.Conn, error) {
	return utils.NewSafeDialContext(dialer, whitelist, func() bool {
		return allowPrivateWebFetchHosts.Load()
	})
}

func newPrivateHostWhitelist(entries []string) (*utils.PrivateHostWhitelist, error) {
	return utils.NewPrivateHostWhitelist(entries)
}

func isPrivateOrRestrictedIP(ip net.IP) bool {
	return utils.IsPrivateOrRestrictedIP(ip)
}
