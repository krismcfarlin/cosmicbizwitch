// Command mcp-proxy speaks the MCP stdio protocol (JSON-RPC 2.0, newline-delimited)
// and forwards tool requests to the cosmicbizwitch server over HTTP.
//
// Environment variables:
//
//	CBW_SERVER_URL  — base URL of the cosmicbizwitch server (default: https://server.cosmicbizwitch.com)
//	CBW_API_TOKEN   — PocketBase superuser Bearer token (optional if credentials provided)
//	CBW_PB_EMAIL    — PocketBase superuser email (used for auto-auth when token absent/expired)
//	CBW_PB_PASSWORD — PocketBase superuser password
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

const defaultServerURL = "https://server.cosmicbizwitch.com"

// tokenManager holds a cached bearer token and re-auths when it expires.
type tokenManager struct {
	mu       sync.Mutex
	serverURL string
	email    string
	password string
	token    string
	expiry   time.Time
}

// get returns a valid bearer token, refreshing via PocketBase auth if needed.
func (tm *tokenManager) get() (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Still valid with 5-minute buffer.
	if tm.token != "" && time.Now().Add(5*time.Minute).Before(tm.expiry) {
		return tm.token, nil
	}
	if tm.email == "" || tm.password == "" {
		if tm.token != "" {
			return tm.token, nil // static token, use as-is
		}
		return "", fmt.Errorf("no credentials configured")
	}

	body, _ := json.Marshal(map[string]string{"identity": tm.email, "password": tm.password})
	resp, err := http.Post(
		tm.serverURL+"/api/collections/_superusers/auth-with-password",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth failed HTTP %d: %s", resp.StatusCode, raw)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.Token == "" {
		return "", fmt.Errorf("auth response parse error: %w", err)
	}
	tm.token = result.Token
	tm.expiry = time.Now().Add(23 * time.Hour)
	return tm.token, nil
}

// rpcRequest is an inbound JSON-RPC 2.0 message from the MCP client.
type rpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"` // nil for notifications
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

// rpcResponse is an outbound JSON-RPC 2.0 response.
type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolCallParams mirrors the tools/call JSON-RPC params.
type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

func main() {
	serverURL := os.Getenv("CBW_SERVER_URL")
	if serverURL == "" {
		serverURL = defaultServerURL
	}

	tm := &tokenManager{
		serverURL: serverURL,
		email:     os.Getenv("CBW_PB_EMAIL"),
		password:  os.Getenv("CBW_PB_PASSWORD"),
		token:     os.Getenv("CBW_API_TOKEN"),
	}
	if tm.token == "" && (tm.email == "" || tm.password == "") {
		os.Exit(1)
	}
	if tm.email != "" && tm.password != "" {
		if _, err := tm.get(); err != nil {
			os.Exit(1)
		}
	}

	enc := json.NewEncoder(os.Stdout)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		// Notifications have no id — do not send a response.
		if req.ID == nil {
			continue
		}

		resp := rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}

		switch req.Method {
		case "initialize":
			resp.Result = map[string]interface{}{
				"protocolVersion": "2025-11-25",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]interface{}{"name": "cosmicbizwitch", "version": "1.0.0"},
			}

		case "ping":
			resp.Result = map[string]interface{}{}

		case "tools/list":
			token, terr := tm.get()
			if terr != nil {
				resp.Error = &rpcError{Code: -32603, Message: fmt.Sprintf("auth error: %v", terr)}
				break
			}
			result, err := proxyGET(serverURL+"/mcp/tools", token)
			if err != nil {
				resp.Error = &rpcError{Code: -32603, Message: fmt.Sprintf("upstream error: %v", err)}
			} else {
				resp.Result = result
			}

		case "tools/call":
			var params toolCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = &rpcError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)}
				break
			}
			token, terr := tm.get()
			if terr != nil {
				resp.Error = &rpcError{Code: -32603, Message: fmt.Sprintf("auth error: %v", terr)}
				break
			}
			body := map[string]interface{}{
				"name":      params.Name,
				"arguments": params.Arguments,
			}
			result, err := proxyPOST(serverURL+"/mcp/call", token, body)
			if err != nil {
				resp.Error = &rpcError{Code: -32603, Message: fmt.Sprintf("upstream error: %v", err)}
			} else {
				resp.Result = result
			}

		default:
			resp.Error = &rpcError{Code: -32601, Message: "method not found"}
		}

		enc.Encode(resp) //nolint
	}
}

// proxyGET performs an authenticated GET and returns the decoded JSON body.
func proxyGET(url, token string) (interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}
	return result, nil
}

// proxyPOST performs an authenticated POST with a JSON body and returns the decoded response.
func proxyPOST(url, token string, payload interface{}) (interface{}, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)
	}

	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}
	return result, nil
}
