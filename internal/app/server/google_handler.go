package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"

	googleapp "cosmicbizwitch/internal/app/google"

	"golang.org/x/oauth2"
)

// handleGoogleAuthStatus returns whether Google Drive is connected.
// GET /api/google/auth/status
func (s *Server) handleGoogleAuthStatus(w http.ResponseWriter, r *http.Request) {
	connected := s.settings.Get("GOOGLE_REFRESH_TOKEN") != ""
	s.respondJSON(w, http.StatusOK, map[string]any{"connected": connected})
}

// handleGoogleAuthStart initiates the OAuth2 flow for Google Drive/Docs.
// It starts a temporary listener on :54321 to catch the callback, then
// redirects the browser to Google's consent page.
// GET /api/google/auth/start
func (s *Server) handleGoogleAuthStart(w http.ResponseWriter, r *http.Request) {
	clientID := s.settings.Get("GOOGLE_CLIENT_ID")
	clientSecret := s.settings.Get("GOOGLE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		http.Error(w, "GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET must be set in Settings before authorizing", http.StatusBadRequest)
		return
	}

	cfg := googleapp.OAuthConfig(clientID, clientSecret)
	state := randomHex(16)
	authURL := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	// Start the callback listener before redirecting, so it is ready
	// to accept the redirect from Google.
	ln, err := net.Listen("tcp", "127.0.0.1:54321")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to start OAuth callback listener on :54321 — is something else using that port? (%v)", err), http.StatusInternalServerError)
		return
	}

	go s.serveOAuthCallback(ln, cfg, state)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// serveOAuthCallback handles exactly one request on ln: the redirect from Google
// containing the authorization code. It exchanges the code for tokens, saves
// the refresh token, rebuilds the Google client, then closes the listener.
func (s *Server) serveOAuthCallback(ln net.Listener, cfg *oauth2.Config, expectedState string) {
	srv := &http.Server{}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always shut down after this one request.
		defer func() { go srv.Shutdown(context.Background()) }()

		q := r.URL.Query()
		if errMsg := q.Get("error"); errMsg != "" {
			http.Error(w, "Google denied access: "+errMsg, http.StatusForbidden)
			s.logger.Printf("google auth: denied by user: %s", errMsg)
			return
		}

		if q.Get("state") != expectedState {
			http.Error(w, "invalid state parameter", http.StatusBadRequest)
			s.logger.Println("google auth: state mismatch in callback")
			return
		}

		code := q.Get("code")
		tok, err := cfg.Exchange(r.Context(), code)
		if err != nil {
			http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
			s.logger.Printf("google auth: token exchange failed: %v", err)
			return
		}

		if tok.RefreshToken == "" {
			http.Error(w, "no refresh token returned — revoke app access in Google account settings and try again", http.StatusInternalServerError)
			s.logger.Println("google auth: no refresh token in response")
			return
		}

		if err := s.settings.Set("GOOGLE_REFRESH_TOKEN", tok.RefreshToken); err != nil {
			http.Error(w, "failed to save refresh token: "+err.Error(), http.StatusInternalServerError)
			s.logger.Printf("google auth: save refresh token: %v", err)
			return
		}
		if err := s.settings.Reload(s.logger); err != nil {
			s.logger.Printf("google auth: settings reload: %v", err)
		}

		s.logger.Println("google auth: successfully connected Google Drive")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body style="font-family:sans-serif;padding:40px;text-align:center">
<h2>✅ Google Drive connected!</h2>
<p>You can close this window and return to the app.</p>
</body></html>`)
	})
	srv.Serve(ln)
}

// handleGoogleDriveBrowse lists files and folders in a Drive directory.
// GET /api/google/drive/browse?parent=<folderID>
func (s *Server) handleGoogleDriveBrowse(w http.ResponseWriter, r *http.Request) {
	gc := s.settings.GoogleClient()
	if gc == nil {
		s.respondJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "Google Drive not configured — visit /api/google/auth/start to connect",
		})
		return
	}

	parentID := r.URL.Query().Get("parent")
	files, err := gc.ListFiles(r.Context(), parentID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "list Drive files failed", err)
		return
	}
	s.respondJSON(w, http.StatusOK, files)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
