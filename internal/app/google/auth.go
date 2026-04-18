package google

import (
	"context"
	"log"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// OAuthConfig builds the oauth2.Config for Google Drive and Docs APIs.
// RedirectURL must match exactly what is registered in Google Cloud Console.
func OAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/drive",
			"https://www.googleapis.com/auth/documents",
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}
}

// TokenManager holds a reusable OAuth2 token source that auto-refreshes the
// access token whenever it expires, using the stored refresh token.
type TokenManager struct {
	ts oauth2.TokenSource
}

// NewTokenManager creates a TokenManager using the given OAuth2 credentials and
// a previously-obtained refresh token. The access token is cached and
// transparently refreshed via oauth2.ReuseTokenSource.
func NewTokenManager(clientID, clientSecret, refreshToken string) *TokenManager {
	return NewTokenManagerWithPersist(clientID, clientSecret, refreshToken, nil)
}

// NewTokenManagerWithPersist is like NewTokenManager but calls onNewRefreshToken
// whenever Google issues a new refresh token during a token refresh cycle.
// This handles refresh token rotation — if Google enables rotation, the old
// refresh token is immediately invalidated and the new one must be persisted.
// Pass nil for onNewRefreshToken to disable persistence (same as NewTokenManager).
func NewTokenManagerWithPersist(clientID, clientSecret, refreshToken string, onNewRefreshToken func(string) error) *TokenManager {
	cfg := OAuthConfig(clientID, clientSecret, "")
	base := cfg.TokenSource(context.Background(), &oauth2.Token{
		RefreshToken: refreshToken,
	})
	reuse := oauth2.ReuseTokenSource(nil, base)

	if onNewRefreshToken == nil {
		return &TokenManager{ts: reuse}
	}

	pts := &persistingTokenSource{
		inner:            reuse,
		lastRefreshToken: refreshToken,
		onNewToken:       onNewRefreshToken,
	}
	return &TokenManager{ts: pts}
}

// persistingTokenSource wraps an oauth2.TokenSource and detects when Google
// issues a new refresh token (rotation). When detected it calls onNewToken so
// the caller can persist the new token before the old one is invalidated.
type persistingTokenSource struct {
	inner            oauth2.TokenSource
	mu               sync.Mutex
	lastRefreshToken string
	onNewToken       func(string) error
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.inner.Token()
	if err != nil {
		return nil, err
	}

	if tok.RefreshToken != "" {
		p.mu.Lock()
		changed := tok.RefreshToken != p.lastRefreshToken
		if changed {
			p.lastRefreshToken = tok.RefreshToken
		}
		p.mu.Unlock()

		if changed {
			if saveErr := p.onNewToken(tok.RefreshToken); saveErr != nil {
				log.Printf("[google/auth] WARNING: new refresh token detected but failed to persist: %v", saveErr)
			} else {
				log.Printf("[google/auth] refresh token rotated — new token persisted to app_settings")
			}
		}
	}

	return tok, nil
}

// GetAccessToken returns a valid access token, refreshing automatically if
// the current one has expired or will expire within the next 10 seconds.
func (tm *TokenManager) GetAccessToken() (string, error) {
	tok, err := tm.ts.Token()
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// TokenExpiry returns the expiry time of the current access token.
// Returns zero time if no token has been fetched yet.
func (tm *TokenManager) TokenExpiry() (time.Time, error) {
	tok, err := tm.ts.Token()
	if err != nil {
		return time.Time{}, err
	}
	return tok.Expiry, nil
}
