package providers

import httpapi "github.com/kiry163/mole/pkg/providers/httpapi"

type (
	HTTPProvider = httpapi.HTTPProvider
)

func NewHTTPProvider(
	apiKey, apiBase, maxTokensField, userAgent string,
	requestTimeoutSeconds int,
	extraBody map[string]any,
	customHeaders map[string]string,
) *HTTPProvider {
	return httpapi.NewHTTPProvider(
		apiKey,
		apiBase,
		maxTokensField,
		userAgent,
		requestTimeoutSeconds,
		extraBody,
		customHeaders,
	)
}
