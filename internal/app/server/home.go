package server

import (
	"fmt"
	"net/http"
)

// handleHome serves the authenticated home page listing available routes.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !s.isDBSessionValid(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	bodyHTML := `
<header>
    <h1>Platform Starter</h1>
    <a href="/logout" class="logout-btn">Logout</a>
</header>
<div class="container">
    <div class="grid" style="grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 24px; padding-top: 20px;">
        <a href="/logs" style="text-decoration: none;">
            <div class="stat-card" style="cursor: pointer; transition: box-shadow 0.2s;" onmouseover="this.style.boxShadow='0 4px 16px rgba(102,126,234,0.3)'" onmouseout="this.style.boxShadow=''">
                <h3>Log Viewer</h3>
                <div class="value" style="font-size: 20px; margin-top: 12px; color: #667eea;">/logs</div>
                <p style="color: #888; font-size: 13px; margin-top: 8px;">Live server log viewer with filtering and auto-refresh</p>
            </div>
        </a>
        <a href="/_/" style="text-decoration: none;">
            <div class="stat-card" style="cursor: pointer; transition: box-shadow 0.2s;" onmouseover="this.style.boxShadow='0 4px 16px rgba(102,126,234,0.3)'" onmouseout="this.style.boxShadow=''">
                <h3>PocketBase Admin UI</h3>
                <div class="value" style="font-size: 20px; margin-top: 12px; color: #667eea;">/_/</div>
                <p style="color: #888; font-size: 13px; margin-top: 8px;">Browse collections, manage data, configure settings</p>
            </div>
        </a>
        <a href="/mcp/tools" style="text-decoration: none;">
            <div class="stat-card" style="cursor: pointer; transition: box-shadow 0.2s;" onmouseover="this.style.boxShadow='0 4px 16px rgba(102,126,234,0.3)'" onmouseout="this.style.boxShadow=''">
                <h3>MCP Tools</h3>
                <div class="value" style="font-size: 20px; margin-top: 12px; color: #667eea;">/mcp/tools</div>
                <p style="color: #888; font-size: 13px; margin-top: 8px;">Generic PocketBase CRUD tools for LLM agents</p>
            </div>
        </a>
        <a href="/health" style="text-decoration: none;">
            <div class="stat-card" style="cursor: pointer; transition: box-shadow 0.2s;" onmouseover="this.style.boxShadow='0 4px 16px rgba(102,126,234,0.3)'" onmouseout="this.style.boxShadow=''">
                <h3>Health Check</h3>
                <div class="value" style="font-size: 20px; margin-top: 12px; color: #667eea;">/health</div>
                <p style="color: #888; font-size: 13px; margin-top: 8px;">Server and database health status</p>
            </div>
        </a>
    </div>
</div>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, baseTemplate("Platform Starter", bodyHTML))
}
