package server

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleLogsPage renders the log viewer page
func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "200"
	}
	filter := r.URL.Query().Get("filter")

	bodyHTML := fmt.Sprintf(`
<header>
	<h1>Server Logs</h1>
	<div style="display: flex; gap: 10px; align-items: center;">
		<a href="/" style="background: #667eea; color: white; padding: 8px 16px; border-radius: 4px; text-decoration: none; font-size: 14px; font-weight: 500;">Home</a>
		<a href="/_/" style="background: #667eea; color: white; padding: 8px 16px; border-radius: 4px; text-decoration: none; font-size: 14px; font-weight: 500;">Admin</a>
		<a href="/logout" class="logout-btn">Logout</a>
	</div>
</header>
<div class="container">
	<div style="background: white; border-radius: 8px; padding: 24px; box-shadow: 0 2px 8px rgba(0,0,0,0.08);">
		<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
			<h2 style="margin: 0;">Server Logs</h2>
			<div style="display: flex; gap: 10px; align-items: center;">
				<label style="font-size: 14px; color: #555;">Lines:
					<select onchange="updateLogs()" id="lines-select" style="padding: 6px; border: 1px solid #dee2e6; border-radius: 4px; margin-left: 4px;">
						<option value="50" %s>50</option>
						<option value="200" %s>200</option>
						<option value="500" %s>500</option>
						<option value="1000" %s>1000</option>
					</select>
				</label>
				<input type="text" id="filter-input" placeholder="Filter..." value="%s"
					style="padding: 6px 10px; border: 1px solid #dee2e6; border-radius: 4px; width: 180px;"
					onkeydown="if(event.key==='Enter') updateLogs()">
				<button onclick="updateLogs()" style="padding: 6px 14px; background: #667eea; color: white; border: none; border-radius: 4px; cursor: pointer; font-weight: 500;">Apply</button>
				<button onclick="toggleAutoRefresh()" id="refresh-btn"
					style="padding: 6px 14px; background: #28a745; color: white; border: none; border-radius: 4px; cursor: pointer; font-weight: 500;">Auto-refresh</button>
			</div>
		</div>
		<div id="log-output"
			hx-get="/logs/data?lines=%s&filter=%s"
			hx-trigger="load"
			hx-swap="innerHTML">
			<div style="text-align: center; padding: 20px; color: #999;">Loading logs...</div>
		</div>
	</div>
</div>
<script>
var refreshTimer = null;
function updateLogs() {
	var lines = document.getElementById('lines-select').value;
	var filter = document.getElementById('filter-input').value;
	window.location = '/logs?lines=' + lines + '&filter=' + encodeURIComponent(filter);
}
function toggleAutoRefresh() {
	var btn = document.getElementById('refresh-btn');
	if (refreshTimer) {
		clearInterval(refreshTimer);
		refreshTimer = null;
		btn.textContent = 'Auto-refresh';
		btn.style.background = '#28a745';
	} else {
		btn.textContent = 'Stop';
		btn.style.background = '#dc3545';
		refreshLines();
		refreshTimer = setInterval(refreshLines, 3000);
	}
}
function refreshLines() {
	var lines = document.getElementById('lines-select').value;
	var filter = document.getElementById('filter-input').value;
	htmx.ajax('GET', '/logs/data?lines=' + lines + '&filter=' + encodeURIComponent(filter), {
		target: '#log-output', swap: 'innerHTML'
	});
}

</script>`,
		selectedIf(lines, "50"),
		selectedIf(lines, "200"),
		selectedIf(lines, "500"),
		selectedIf(lines, "1000"),
		html.EscapeString(filter),
		lines, filter,
	)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, baseTemplate("Server Logs", bodyHTML))
}

// handleLogsAPI returns log lines as HTML from the in-memory buffer
func (s *Server) handleLogsAPI(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("lines"))
	if err != nil || n < 1 || n > 5000 {
		n = 200
	}
	filter := r.URL.Query().Get("filter")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if s.logBuffer == nil {
		fmt.Fprint(w, `<div style="color: #dcdcaa; padding: 10px; font-family: monospace; background: #1e1e1e; border-radius: 6px;">Log buffer not initialized. Restart the server to enable log capture.</div>`)
		return
	}

	lines := s.logBuffer.Lines(n, filter)

	// Reverse so newest is at the top
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	fmt.Fprint(w, `<div style="font-family: monospace; font-size: 12px; background: #1e1e1e; color: #d4d4d4; padding: 16px; border-radius: 6px; overflow-y: auto; max-height: 72vh; white-space: pre-wrap; word-break: break-all;">`)
	fmt.Fprintf(w, `<div style="color: #555; margin-bottom: 8px; font-size: 11px;">— %d lines as of %s —</div>`, len(lines), time.Now().Format("15:04:05"))

	for _, line := range lines {
		color := logLineColor(line.Text)
		fmt.Fprintf(w,
			`<div style="color: %s; line-height: 1.5;"><span style="color: #555; user-select: none;">%s </span>%s</div>`,
			color,
			html.EscapeString(line.Time),
			html.EscapeString(line.Text),
		)
	}

	if len(lines) == 0 {
		fmt.Fprint(w, `<div style="color: #888;">No log lines captured yet.</div>`)
	}

	fmt.Fprint(w, `</div>`)
}

// logLineColor returns a CSS color for a log line based on its content
func logLineColor(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "fatal"):
		return "#f48771"
	case strings.Contains(lower, "warn"):
		return "#dcdcaa"
	case strings.Contains(lower, "success") || strings.Contains(lower, "complete"):
		return "#4ec9b0"
	case strings.Contains(lower, "start"):
		return "#9cdcfe"
	default:
		return "#d4d4d4"
	}
}

func selectedIf(current, value string) string {
	if current == value {
		return `selected="selected"`
	}
	return ""
}
