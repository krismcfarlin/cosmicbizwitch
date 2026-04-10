package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const slackOAuthCallbackURL = "https://server.cosmicbizwitch.com/api/slack/auth/callback"

// handleSlackAuthStatus returns whether Slack is connected.
// GET /api/slack/auth/status
func (s *Server) handleSlackAuthStatus(w http.ResponseWriter, r *http.Request) {
	connected := s.settings.Get("SLACK_BOT_TOKEN") != ""
	workspace := s.settings.Get("SLACK_WORKSPACE_NAME")
	s.respondJSON(w, http.StatusOK, map[string]any{
		"connected": connected,
		"workspace": workspace,
	})
}

// getSlackOAuthCredentials returns Slack OAuth credentials from settings.
func (s *Server) getSlackOAuthCredentials() (clientID, clientSecret string) {
	clientID = s.settings.Get("SLACK_CLIENT_ID")
	clientSecret = s.settings.Get("SLACK_CLIENT_SECRET")
	return
}

// handleSlackAuthStart initiates the OAuth2 flow for Slack.
// GET /api/slack/auth/start
func (s *Server) handleSlackAuthStart(w http.ResponseWriter, r *http.Request) {
	clientID, _ := s.getSlackOAuthCredentials()
	if clientID == "" {
		http.Error(w, "SLACK_CLIENT_ID not configured — add it in Settings", http.StatusBadRequest)
		return
	}

	state := randomHex(16)

	s.oauthStateMu.Lock()
	s.oauthPending[state] = pendingOAuth{
		cfg:       nil, // Slack does not use oauth2.Config; we store nil and use state only
		expiresAt: time.Now().Add(10 * time.Minute),
	}
	s.oauthStateMu.Unlock()

	params := url.Values{
		"client_id":    {clientID},
		"scope":        {"chat:write,channels:read,groups:read"},
		"redirect_uri": {slackOAuthCallbackURL},
		"state":        {state},
	}
	authURL := "https://slack.com/oauth/v2/authorize?" + params.Encode()
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleSlackAuthCallback receives the redirect from Slack after the user grants access.
// GET /api/slack/auth/callback
func (s *Server) handleSlackAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if errMsg := q.Get("error"); errMsg != "" {
		http.Error(w, "Slack denied access: "+errMsg, http.StatusForbidden)
		s.logger.Printf("slack auth: denied by user: %s", errMsg)
		return
	}

	state := q.Get("state")
	s.oauthStateMu.Lock()
	pending, ok := s.oauthPending[state]
	if ok {
		delete(s.oauthPending, state)
	}
	s.oauthStateMu.Unlock()

	if !ok || time.Now().After(pending.expiresAt) {
		http.Error(w, "invalid or expired state parameter", http.StatusBadRequest)
		s.logger.Println("slack auth: invalid or expired state in callback")
		return
	}

	clientID, clientSecret := s.getSlackOAuthCredentials()
	if clientID == "" || clientSecret == "" {
		http.Error(w, "Slack OAuth credentials not configured in Settings", http.StatusInternalServerError)
		s.logger.Println("slack auth: SLACK_CLIENT_ID or SLACK_CLIENT_SECRET missing")
		return
	}

	// Exchange code for bot token via Slack's oauth.v2.access endpoint.
	code := q.Get("code")
	tokenResp, err := s.slackExchangeCode(r.Context(), clientID, clientSecret, code)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
		s.logger.Printf("slack auth: token exchange failed: %v", err)
		return
	}

	if err := s.settings.Set("SLACK_BOT_TOKEN", tokenResp.botToken); err != nil {
		http.Error(w, "failed to save bot token: "+err.Error(), http.StatusInternalServerError)
		s.logger.Printf("slack auth: save bot token: %v", err)
		return
	}
	if tokenResp.workspaceName != "" {
		if err := s.settings.Set("SLACK_WORKSPACE_NAME", tokenResp.workspaceName); err != nil {
			s.logger.Printf("slack auth: save workspace name: %v", err)
		}
	}

	if err := s.settings.Reload(s.logger); err != nil {
		s.logger.Printf("slack auth: settings reload: %v", err)
	}

	s.logger.Printf("slack auth: successfully connected workspace %q", tokenResp.workspaceName)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html><body style="font-family:sans-serif;padding:40px;text-align:center">
<h2>Slack connected!</h2>
<p>You can close this window and return to the app.</p>
</body></html>`)
}

// slackTokenResponse holds the fields we care about from oauth.v2.access.
type slackTokenResponse struct {
	botToken      string
	workspaceName string
}

// slackExchangeCode calls Slack's oauth.v2.access endpoint and returns the bot token.
func (s *Server) slackExchangeCode(ctx context.Context, clientID, clientSecret, code string) (slackTokenResponse, error) {
	form := url.Values{
		"code":         {code},
		"redirect_uri": {slackOAuthCallbackURL},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://slack.com/api/oauth.v2.access",
		strings.NewReader(form.Encode()))
	if err != nil {
		return slackTokenResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return slackTokenResponse{}, fmt.Errorf("POST oauth.v2.access: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var parsed struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Team  struct {
			Name string `json:"name"`
		} `json:"team"`
		AccessToken string `json:"access_token"`
		AuthedUser  struct {
			AccessToken string `json:"access_token"`
		} `json:"authed_user"`
		BotToken string `json:"bot_token"`
		// oauth.v2.access may return the bot token under different keys
		// depending on the Slack app type. We check both.
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return slackTokenResponse{}, fmt.Errorf("parse token response: %w", err)
	}
	if !parsed.OK {
		return slackTokenResponse{}, fmt.Errorf("Slack API error: %s", parsed.Error)
	}

	// For OAuth v2, the bot token is in access_token when the app has a bot scope.
	botToken := parsed.AccessToken
	if botToken == "" {
		botToken = parsed.BotToken
	}
	if botToken == "" {
		return slackTokenResponse{}, fmt.Errorf("no bot token in Slack OAuth response")
	}

	return slackTokenResponse{
		botToken:      botToken,
		workspaceName: parsed.Team.Name,
	}, nil
}
