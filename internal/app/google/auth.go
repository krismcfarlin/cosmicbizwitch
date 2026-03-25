package google

import (
	"context"

	"golang.org/x/oauth2"
)

// OAuthConfig builds the oauth2.Config for Google Drive and Docs APIs.
// RedirectURL must match exactly what is registered in Google Cloud Console.
func OAuthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "http://127.0.0.1:54321/",
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
	cfg := OAuthConfig(clientID, clientSecret)
	// Seed with a token that only has the refresh token set.
	// oauth2 will exchange it for an access token on first use.
	base := cfg.TokenSource(context.Background(), &oauth2.Token{
		RefreshToken: refreshToken,
	})
	return &TokenManager{ts: oauth2.ReuseTokenSource(nil, base)}
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
