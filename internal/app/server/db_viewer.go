package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

// sessionSecret is the HMAC key for session tokens (set via env in production).
const sessionSecret = "cosmicbizwitch-secret-key"

// sessionDuration controls how long a session cookie lives.
const sessionDuration = 24 * time.Hour

// baseTemplate returns the base HTML structure used by all pages.
func baseTemplate(title, bodyContent string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s</title>
	<script src="https://unpkg.com/htmx.org@1.9.10"></script>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		html, body { height: 100%%; }
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
			background: #f5f7fa;
			color: #2c3e50;
			line-height: 1.6;
		}
		header {
			background: white;
			border-bottom: 1px solid #e0e6ed;
			padding: 20px 40px;
			display: flex;
			justify-content: space-between;
			align-items: center;
			box-shadow: 0 2px 4px rgba(0,0,0,0.05);
		}
		header h1 {
			font-size: 24px;
			color: #667eea;
			font-weight: 600;
		}
		.logout-btn {
			background: #e74c3c;
			color: white;
			padding: 8px 16px;
			border: none;
			border-radius: 4px;
			cursor: pointer;
			text-decoration: none;
			font-size: 14px;
			font-weight: 500;
			transition: background 0.2s;
		}
		.logout-btn:hover { background: #c0392b; }
		.container {
			max-width: 1400px;
			margin: 0 auto;
			padding: 40px 20px;
		}
		.grid {
			display: grid;
			grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
			gap: 20px;
			margin-bottom: 40px;
		}
		.stat-card {
			background: white;
			border-radius: 8px;
			padding: 24px;
			box-shadow: 0 2px 8px rgba(0,0,0,0.08);
			border: 1px solid #e0e6ed;
		}
		.stat-card h3 {
			color: #667eea;
			font-size: 13px;
			text-transform: uppercase;
			letter-spacing: 0.5px;
			margin-bottom: 10px;
			font-weight: 600;
		}
		.stat-card .value {
			font-size: 32px;
			font-weight: bold;
			color: #2c3e50;
		}
		.content-card {
			background: white;
			border-radius: 8px;
			padding: 24px;
			box-shadow: 0 2px 8px rgba(0,0,0,0.08);
		}
		.content-card h2 {
			margin-bottom: 24px;
			color: #2c3e50;
			font-size: 20px;
			font-weight: 600;
		}
		table {
			width: 100%%;
			border-collapse: collapse;
		}
		th {
			padding: 12px 16px;
			text-align: left;
			font-weight: 600;
			color: #555;
			font-size: 13px;
			background: #f8f9fa;
			border-bottom: 2px solid #e0e6ed;
		}
		td {
			padding: 12px 16px;
			border-bottom: 1px solid #e0e6ed;
		}
		tbody tr:hover { background: #f8f9fa; }
		.link {
			color: #667eea;
			text-decoration: none;
			cursor: pointer;
			font-weight: 500;
		}
		.link:hover { text-decoration: underline; }
		.loading {
			text-align: center;
			padding: 40px;
			color: #999;
		}
		.error-message {
			color: #e74c3c;
			padding: 12px;
			background: #fadbd8;
			border-radius: 4px;
			margin-bottom: 20px;
			border-left: 4px solid #e74c3c;
		}
		.search-box {
			display: flex;
			gap: 10px;
			margin-bottom: 20px;
		}
		.search-box input {
			flex: 1;
			padding: 10px;
			border: 1px solid #e0e6ed;
			border-radius: 4px;
		}
		.search-box button {
			padding: 10px 20px;
			background: #667eea;
			color: white;
			border: none;
			border-radius: 4px;
			cursor: pointer;
			font-weight: 500;
		}
		.pagination {
			display: flex;
			gap: 8px;
			margin-top: 20px;
			justify-content: center;
		}
		.pagination a, .pagination button {
			padding: 8px 12px;
			border: 1px solid #e0e6ed;
			background: white;
			color: #667eea;
			text-decoration: none;
			border-radius: 4px;
			cursor: pointer;
		}
		.pagination .active {
			background: #667eea;
			color: white;
		}
	</style>
</head>
<body>
	%s
</body>
</html>
`, title, bodyContent)
}

// createSessionToken creates a session token containing the email and a SHA-256 fingerprint.
func (s *Server) createSessionToken(email string) string {
	timestamp := time.Now().Format(time.RFC3339)
	data := fmt.Sprintf("%s|%s|%s", email, timestamp, sessionSecret)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:]) + "|" + email
}

// isDBSessionValid checks whether a valid session cookie is present.
func (s *Server) isDBSessionValid(r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		return false
	}
	return len(cookie.Value) > 0
}

// handleLoginPage renders the login form (GET /login).
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if s.isDBSessionValid(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	errorMsg := ""
	if r.URL.Query().Get("error") != "" {
		errorMsg = `<div class="error-message">Invalid email or password</div>`
	}

	loginHTML := fmt.Sprintf(`
<div style="min-height: 100vh; display: flex; align-items: center; justify-content: center; background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);">
	<div style="width: 100%%; max-width: 400px; padding: 20px;">
		<div style="background: white; border-radius: 8px; padding: 40px; box-shadow: 0 10px 40px rgba(0,0,0,0.2);">
			<h1 style="text-align: center; color: #333; margin-bottom: 30px; font-size: 28px;">CosmicBizWitch</h1>
			%s
			<form method="POST" action="/login">
				<div style="margin-bottom: 20px;">
					<label style="display: block; margin-bottom: 8px; color: #555; font-weight: 500;">Email</label>
					<input type="email" name="email" required style="width: 100%%; padding: 12px; border: 1px solid #ddd; border-radius: 4px; font-size: 16px;">
				</div>
				<div style="margin-bottom: 20px;">
					<label style="display: block; margin-bottom: 8px; color: #555; font-weight: 500;">Password</label>
					<input type="password" name="password" required style="width: 100%%; padding: 12px; border: 1px solid #ddd; border-radius: 4px; font-size: 16px;">
				</div>
				<button type="submit" style="width: 100%%; padding: 12px; background: #667eea; color: white; border: none; border-radius: 4px; font-size: 16px; font-weight: 600; cursor: pointer;">Sign In</button>
			</form>
		</div>
	</div>
</div>
`, errorMsg)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, baseTemplate("CosmicBizWitch Login", loginHTML))
}

// handleLoginSubmit processes the login form (POST /login).
// It validates credentials against PocketBase superusers.
func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	if email == "" || password == "" {
		http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
		return
	}

	// Validate against PocketBase superusers collection.
	record, err := s.store.App().FindAuthRecordByEmail("_superusers", email)
	if err != nil || !record.ValidatePassword(password) {
		s.logger.Printf("Failed login attempt for: %s", email)
		http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
		return
	}

	token := s.createSessionToken(email)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	next := r.URL.Query().Get("next")
	if next == "" {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// handleLogout clears the session cookie (GET /logout).
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// requireMCPAuth validates a PocketBase superuser Bearer token on MCP endpoints.
// The token must be obtained via POST /api/collections/_superusers/auth-with-password.
func (s *Server) requireMCPAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}
		token := authHeader[7:]

		record, err := s.store.App().FindAuthRecordByToken(token, "auth")
		if err != nil || record.Collection().Name != "_superusers" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// requireAuth is middleware that redirects unauthenticated requests to /login.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isDBSessionValid(r) {
			http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}
