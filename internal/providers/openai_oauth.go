package providers

// ChatGPT "Sign in with ChatGPT" OAuth flow (mirrors the OpenAI Codex CLI).
// Lets a ChatGPT Plus/Pro/Team subscription authorize agent inference through
// the ChatGPT backend, instead of an API-key-billed OPENAI_API_KEY.
//
// Flow: PKCE (S256) authorize at auth.openai.com -> loopback callback on
// localhost:1455 -> token exchange -> account id decoded from the id_token JWT
// claim "https://api.openai.com/auth".chatgpt_account_id. Credentials are
// stored in ~/.torus/openai_auth.json and refreshed proactively.
//
// NOTE: this reuses generatePKCE(), base64URLEncode() and tokenRefreshBuffer
// defined in oauth.go (same package).

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	openaiOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	openaiAuthorizeURL  = "https://auth.openai.com/oauth/authorize"
	openaiTokenURL      = "https://auth.openai.com/oauth/token"
	// Scopes match the Codex CLI so the streamlined consent screen is used and
	// the id_token carries the org/account claims (offline_access -> refresh token).
	openaiOAuthScopes  = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	openaiOriginator   = "codex_cli_rs"
	openaiCallbackPath = "/auth/callback"
)

// openaiCallbackPorts is tried in order; the redirect_uri must match the bound port.
var openaiCallbackPorts = []int{1455, 1457}

// OpenAICredentials holds ChatGPT OAuth tokens plus the derived account id.
type OpenAICredentials struct {
	Access    string `json:"access"`
	Refresh   string `json:"refresh"`
	IDToken   string `json:"id_token"`
	AccountID string `json:"account_id"`
	ExpiresAt int64  `json:"expires_at"` // unix ms
}

func openaiAuthPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".torus", "openai_auth.json")
}

// IsOpenAIOAuthToken reports whether a key is a ChatGPT OAuth access token (a
// JWT) rather than a standard sk- API key.
func IsOpenAIOAuthToken(key string) bool {
	if strings.HasPrefix(key, "sk-") {
		return false
	}
	return strings.HasPrefix(key, "eyJ") && strings.Count(key, ".") == 2
}

// GetOpenAIAccessToken returns a valid ChatGPT access token, refreshing if needed.
func GetOpenAIAccessToken() (string, error) {
	creds, err := GetOpenAIAuth()
	if err != nil {
		return "", err
	}
	return creds.Access, nil
}

// GetOpenAIAuth loads stored credentials, refreshing + persisting when expiring.
func GetOpenAIAuth() (*OpenAICredentials, error) {
	creds, err := LoadOpenAICredentials()
	if err != nil {
		return nil, err
	}
	return ensureFreshOpenAI(creds), nil
}

// OpenAIAccountID returns the stored ChatGPT account id (or "" if none).
func OpenAIAccountID() string {
	creds, err := LoadOpenAICredentials()
	if err != nil {
		return ""
	}
	return creds.AccountID
}

func ensureFreshOpenAI(creds *OpenAICredentials) *OpenAICredentials {
	if creds.ExpiresAt != 0 && time.Now().UnixMilli() < creds.ExpiresAt-tokenRefreshBuffer {
		return creds
	}
	if creds.Refresh == "" {
		return creds // cannot refresh; use as-is
	}
	refreshed, err := RefreshOpenAIToken(creds.Refresh)
	if err != nil {
		log.Printf("[openai-oauth] warning: token refresh failed, using existing token: %v", err)
		return creds
	}
	// The refresh response may omit fields we already have.
	if refreshed.AccountID == "" {
		refreshed.AccountID = creds.AccountID
	}
	if refreshed.Refresh == "" {
		refreshed.Refresh = creds.Refresh
	}
	if err := SaveOpenAICredentials(refreshed); err != nil {
		log.Printf("[openai-oauth] warning: could not persist refreshed credentials: %v", err)
	}
	return refreshed
}

// ── Login flow ────────────────────────────────────────────────────────────────

// LoginOpenAI runs the "Sign in with ChatGPT" OAuth PKCE flow using a local
// loopback callback server. onAuthURL is called with the URL to open.
func LoginOpenAI(onAuthURL func(string)) (*OpenAICredentials, error) {
	verifier, challenge := generatePKCE()

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed to generate OAuth state: %w", err)
	}
	expectedState := base64URLEncode(stateBytes)

	// Bind the loopback callback server (default port, then fallback).
	var listener net.Listener
	var port int
	var lastErr error
	for _, cand := range openaiCallbackPorts {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cand))
		if err == nil {
			listener, port = l, cand
			break
		}
		lastErr = err
	}
	if listener == nil {
		return nil, fmt.Errorf("could not bind loopback callback port %v: %w", openaiCallbackPorts, lastErr)
	}
	redirectURI := fmt.Sprintf("http://localhost:%d%s", port, openaiCallbackPath)

	type cbResult struct {
		code, state string
		err         error
	}
	resultCh := make(chan cbResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(openaiCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			http.Error(w, "Login failed: "+e, http.StatusBadRequest)
			resultCh <- cbResult{err: fmt.Errorf("authorization error: %s (%s)", e, q.Get("error_description"))}
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, openaiLoginSuccessHTML)
		resultCh <- cbResult{code: q.Get("code"), state: q.Get("state")}
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	onAuthURL(buildOpenAIAuthorizeURL(challenge, expectedState, redirectURI))

	var res cbResult
	select {
	case res = <-resultCh:
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("timed out waiting for ChatGPT OAuth callback")
	}
	if res.err != nil {
		return nil, res.err
	}
	if res.code == "" {
		return nil, fmt.Errorf("no authorization code in callback")
	}
	if res.state != expectedState {
		return nil, fmt.Errorf("OAuth state mismatch: possible CSRF (got %q, want %q)", res.state, expectedState)
	}

	return exchangeOpenAICode(res.code, verifier, redirectURI)
}

func buildOpenAIAuthorizeURL(challenge, state, redirectURI string) string {
	params := url.Values{
		"response_type":              {"code"},
		"client_id":                  {openaiOAuthClientID},
		"redirect_uri":               {redirectURI},
		"scope":                      {openaiOAuthScopes},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"originator":                 {openaiOriginator},
		"state":                      {state},
	}
	return openaiAuthorizeURL + "?" + params.Encode()
}

// ── Token exchange / refresh ──────────────────────────────────────────────────

func exchangeOpenAICode(code, verifier, redirectURI string) (*OpenAICredentials, error) {
	return postOpenAIToken(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {openaiOAuthClientID},
		"code_verifier": {verifier},
	})
}

// RefreshOpenAIToken exchanges a refresh token for a fresh access token.
func RefreshOpenAIToken(refreshToken string) (*OpenAICredentials, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("cannot refresh: empty refresh token")
	}
	return postOpenAIToken(url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {openaiOAuthClientID},
		"refresh_token": {refreshToken},
		"scope":         {openaiOAuthScopes},
	})
}

func postOpenAIToken(form url.Values) (*OpenAICredentials, error) {
	resp, err := http.PostForm(openaiTokenURL, form)
	if err != nil {
		return nil, fmt.Errorf("token endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned empty access token")
	}
	expiresIn := tr.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600 // conservative default -> forces a refresh soon
	}
	creds := &OpenAICredentials{
		Access:    tr.AccessToken,
		Refresh:   tr.RefreshToken,
		IDToken:   tr.IDToken,
		AccountID: accountIDFromIDToken(tr.IDToken),
		ExpiresAt: time.Now().UnixMilli() + expiresIn*1000,
	}
	return creds, nil
}

// accountIDFromIDToken decodes the JWT id_token and reads the account id from
// the claim path "https://api.openai.com/auth" -> chatgpt_account_id.
func accountIDFromIDToken(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		if payload, err = base64.URLEncoding.DecodeString(padBase64(parts[1])); err != nil {
			return ""
		}
	}
	var claims struct {
		Auth struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Auth.ChatGPTAccountID
}

func padBase64(s string) string {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return s
}

// ── Credential storage ────────────────────────────────────────────────────────

func LoadOpenAICredentials() (*OpenAICredentials, error) {
	path := openaiAuthPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read openai credentials from %s: %w", path, err)
	}
	var creds OpenAICredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse openai credentials from %s: %w", path, err)
	}
	return &creds, nil
}

func SaveOpenAICredentials(creds *OpenAICredentials) error {
	dir := filepath.Dir(openaiAuthPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create auth directory: %w", err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal openai credentials: %w", err)
	}
	return os.WriteFile(openaiAuthPath(), data, 0600)
}

const openaiLoginSuccessHTML = `<!doctype html><html><body style="font-family:sans-serif;text-align:center;padding-top:80px">` +
	`<h2>&#10003; Signed in to ChatGPT</h2><p>You can close this tab and return to the terminal.</p></body></html>`
