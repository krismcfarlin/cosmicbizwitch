package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"time"

	googleapp "cosmicbizwitch/internal/app/google"

	"golang.org/x/oauth2"
)

const googleOAuthCallbackURL = "https://server.cosmicbizwitch.com/api/google/auth/callback"

// pendingOAuth holds state for an in-progress OAuth flow.
type pendingOAuth struct {
	cfg       *oauth2.Config
	expiresAt time.Time
}

// handleGoogleAuthStatus returns whether Google Drive is connected.
// GET /api/google/auth/status
func (s *Server) handleGoogleAuthStatus(w http.ResponseWriter, r *http.Request) {
	connected := s.settings.Get("GOOGLE_REFRESH_TOKEN") != ""
	s.respondJSON(w, http.StatusOK, map[string]any{"connected": connected})
}

// getGoogleOAuthCredentials returns OAuth credentials from settings, environment, or built-in defaults.
func (s *Server) getGoogleOAuthCredentials() (clientID, clientSecret string) {
	clientID = s.settings.Get("GOOGLE_CLIENT_ID")
	if clientID == "" {
		clientID = os.Getenv("GOOGLE_CLIENT_ID")
	}
	clientSecret = s.settings.Get("GOOGLE_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	}
	return
}

// handleGoogleAuthStart initiates the OAuth2 flow for Google Drive/Docs.
// GET /api/google/auth/start
func (s *Server) handleGoogleAuthStart(w http.ResponseWriter, r *http.Request) {
	clientID, clientSecret := s.getGoogleOAuthCredentials()

	cfg := googleapp.OAuthConfig(clientID, clientSecret, googleOAuthCallbackURL)
	state := randomHex(16)

	s.oauthStateMu.Lock()
	s.oauthPending[state] = pendingOAuth{cfg: cfg, expiresAt: time.Now().Add(10 * time.Minute)}
	s.oauthStateMu.Unlock()

	authURL := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleGoogleAuthCallback receives the redirect from Google after the user grants access.
// GET /api/google/auth/callback
func (s *Server) handleGoogleAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if errMsg := q.Get("error"); errMsg != "" {
		http.Error(w, "Google denied access: "+errMsg, http.StatusForbidden)
		s.logger.Printf("google auth: denied by user: %s", errMsg)
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
		s.logger.Println("google auth: invalid or expired state in callback")
		return
	}

	tok, err := pending.cfg.Exchange(r.Context(), q.Get("code"))
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

	// Log file/folder IDs for easy copying to test env vars
	for _, f := range files {
		if f.MimeType == "application/vnd.google-apps.folder" {
			s.logger.Printf("[TEST_DEST_FOLDER_ID] %s: %s", f.Name, f.ID)
		} else if f.MimeType == "application/vnd.google-apps.document" {
			s.logger.Printf("[TEST_TEMPLATE_DOC_ID] %s: %s", f.Name, f.ID)
		}
	}

	s.respondJSON(w, http.StatusOK, files)
}

// handleGdriveGetContent fetches the text content of a Google Doc.
// GET /api/workflows/gdrive-content?file_id=<docID>
func (s *Server) handleGdriveGetContent(w http.ResponseWriter, r *http.Request) {
	gc := s.settings.GoogleClient()
	if gc == nil {
		s.respondJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "Google Drive not configured",
		})
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]any{
			"error": "missing file_id parameter",
		})
		return
	}

	content, err := gc.GetDocumentContent(r.Context(), fileID)
	if err != nil {
		s.logger.Printf("gdrive get content %s: %v", fileID, err)
		s.respondJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "failed to fetch document content",
		})
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"text": content,
	})
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
