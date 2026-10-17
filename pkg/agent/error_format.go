package agent

import (
	"fmt"

	"github.com/kiry163/mole/pkg/providers"
)

func formatProcessingError(err error) string {
	if err == nil {
		return ""
	}

	if kind, ok := providers.ClassifyAuthError(err); ok {
		return fmt.Sprintf(
			"Error processing message: %s\n\nOriginal error:\n%s",
			authErrorFriendlyMessage(kind),
			err.Error(),
		)
	}

	return fmt.Sprintf("Error processing message: %v", err)
}

func authErrorFriendlyMessage(kind providers.AuthErrorKind) string {
	switch kind {
	case providers.AuthErrorInvalidAPIKey:
		return "Authentication failed: the API key appears to be invalid. Check the API key configured for this model or provider."
	case providers.AuthErrorMissingAPIKey:
		return "Authentication failed: no API key is configured for this model or provider. Add an API key in the model settings or config."
	case providers.AuthErrorExpiredToken:
		return "Authentication failed: the saved token appears to be expired. Refresh the credential configured for this model or provider."
	default:
		return "Authentication failed: check the API key or provider permissions for this model."
	}
}
