package dashboard

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/dashboard/api"
)

// tokenStore keeps the console's login token in the config's dashboard section.
// The token is a SecureString, so SaveConfig promotes it to DASHBOARD_TOKEN in
// the instance .env and config.json keeps a $DASHBOARD_TOKEN reference; this
// type is the only place that reads the resolved value back.
type tokenStore struct {
	configPath string
}

func newTokenStore(configPath string) tokenStore {
	return tokenStore{configPath: configPath}
}

// jsonMask is what SecureString hides secrets behind in JSON views. It must
// never be accepted as a credential: it is public knowledge, so a leftover copy
// in config.json would be a guessable token.
const jsonMask = "[NOT_HERE]"

// token returns the configured token, or "" when none is set. A whitespace-only
// value counts as unset, matching every other SecureString in the config, and so
// does a leftover "[NOT_HERE]" mask.
func (s tokenStore) token(_ context.Context) (string, error) {
	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	tok := strings.TrimSpace(cfg.Dashboard.Token.String())
	if tok == jsonMask {
		return "", nil
	}
	return tok, nil
}

// IsInitialized reports whether a console token has been configured.
func (s tokenStore) IsInitialized(ctx context.Context) (bool, error) {
	token, err := s.token(ctx)
	if err != nil {
		return false, err
	}
	return token != "", nil
}

// SetToken stores tok as the console token. SaveConfig promotes it to
// DASHBOARD_TOKEN in .env; config.json keeps only the reference.
func (s tokenStore) SetToken(ctx context.Context, tok string) error {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return errors.New("dashboard token must not be empty")
	}
	if tok == jsonMask {
		// Reads treat the mask as "no token set", so storing it would report
		// success while leaving the console unconfigured.
		return fmt.Errorf("%w: must not be the JSON mask %q", api.ErrInvalidToken, jsonMask)
	}
	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.Dashboard.Token = *config.NewSecureString(tok)
	if err := config.SaveConfig(s.configPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// VerifyToken reports whether tok matches the configured token, comparing in
// constant time so the comparison leaks nothing about the stored value.
func (s tokenStore) VerifyToken(ctx context.Context, tok string) (bool, error) {
	want, err := s.token(ctx)
	if err != nil {
		return false, err
	}
	if want == "" || tok == "" {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(tok)) == 1, nil
}
