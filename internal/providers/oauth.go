package providers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	oauthClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	oauthAuthorizeURL = "https://claude.ai/oauth/authorize"
	oauthTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	oauthRedirectURI  = "https://console.anthropic.com/oauth/code/callback"
	oauthScopes       = "org:create_api_key user:profile user:inference"

	// tokenRefreshBuffer is the time before expiry to trigger a proactive refresh.
	tokenRefreshBuffer = 5 * 60 * 1000 // 5 minutes in milliseconds
)

// OAuthCredentials holds the OAuth tokens.
type OAuthCredentials struct {
	Access    string `json:"access"`
	Refresh   string `json:"refresh"`
	ExpiresAt int64  `json:"expires_at"` // unix ms
}

// authDir returns the credential storage file path.
func authDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".torus", "auth.json")
}

// hasAuth checks if OAuth credentials or ANTHROPIC_API_KEY exist.
func hasAuth() bool {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return true
	}
	_, err := os.ReadFile(authDir())
	return err == nil
}

// GetAnthropicKey returns the best available Anthropic key.
// Priority: env var -> OAuth token (auto-refreshed if expired).
func GetAnthropicKey() (string, error) {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key, nil
	}
	creds, err := loadCredentials()
	if err != nil {
		return "", fmt.Errorf("no API key and no OAuth credentials: %w", err)
	}
	if isTokenExpiring(creds) {
		refreshed, err := RefreshToken(creds.Refresh)
		if err != nil {
			log.Printf("[oauth] warning: token refresh failed, credentials may be stale: %v", err)
			return "", fmt.Errorf("token refresh failed: %w", err)
		}
		creds = refreshed
		if err := saveCredentials(creds); err != nil {
			log.Printf("[oauth] warning: could not persist refreshed credentials: %v", err)
		}
	}
	return creds.Access, nil
}

// isTokenExpiring reports whether the token is expired or within the refresh buffer.
func isTokenExpiring(creds *OAuthCredentials) bool {
	return time.Now().UnixMilli() >= creds.ExpiresAt-tokenRefreshBuffer
}

// IsOAuthToken checks if a key is an OAuth access token.
func IsOAuthToken(key string) bool {
	return strings.Contains(key, "sk-ant-oat")
}

// ── PKCE ────────────────────────────────────────────────────────────────────

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func generatePKCE() (verifier, challenge string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Printf("[oauth] warning: crypto/rand read failed: %v", err)
	}
	verifier = base64URLEncode(b)
	hash := sha256.Sum256([]byte(verifier))
	challenge = base64URLEncode(hash[:])
	return
}

// ── Login flow ──────────────────────────────────────────────────────────────

// buildAuthorizeURL constructs the PKCE authorize URL with the given challenge.
func buildAuthorizeURL(challenge string) string {
	params := url.Values{
		"code":                  {"true"},
		"client_id":             {oauthClientID},
		"response_type":         {"code"},
		"redirect_uri":          {oauthRedirectURI},
		"scope":                 {oauthScopes},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return oauthAuthorizeURL + "?" + params.Encode()
}

// parseCodeState splits a "code#state" string into its components.
// Returns the code and state; state is empty if the separator is missing.
func parseCodeState(raw string) (code, state string) {
	parts := strings.SplitN(raw, "#", 2)
	code = parts[0]
	if len(parts) > 1 {
		state = parts[1]
	}
	return
}

// LoginAnthropic runs the OAuth PKCE flow.
// onAuthURL is called with the URL to open in browser.
// onPromptCode is called to get the pasted "code#state" from the user.
func LoginAnthropic(onAuthURL func(string), onPromptCode func() (string, error)) (*OAuthCredentials, error) {
	verifier, challenge := generatePKCE()

	authURL := buildAuthorizeURL(challenge)
	onAuthURL(authURL)

	codeState, err := onPromptCode()
	if err != nil {
		return nil, fmt.Errorf("failed to read authorization code from user: %w", err)
	}

	code, state := parseCodeState(codeState)
	if code == "" {
		return nil, fmt.Errorf("empty authorization code received")
	}

	creds, err := exchangeCode(code, state, verifier)
	if err != nil {
		log.Printf("[oauth] warning: code exchange failed for state=%q: %v", state, err)
		return nil, err
	}
	return creds, nil
}

// postTokenRequest sends a JSON payload to the token endpoint and decodes the response.
func postTokenRequest(payload map[string]string) (*OAuthCredentials, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token request: %w", err)
	}

	resp, err := http.Post(oauthTokenURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("token endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]any
		if decErr := json.NewDecoder(resp.Body).Decode(&errBody); decErr != nil {
			log.Printf("[oauth] warning: could not decode error response body: %v", decErr)
		}
		return nil, fmt.Errorf("token endpoint returned HTTP %d: %v", resp.StatusCode, errBody)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned empty access token")
	}

	return &OAuthCredentials{
		Access:    tokenResp.AccessToken,
		Refresh:   tokenResp.RefreshToken,
		ExpiresAt: time.Now().UnixMilli() + tokenResp.ExpiresIn*1000 - tokenRefreshBuffer,
	}, nil
}

func exchangeCode(code, state, verifier string) (*OAuthCredentials, error) {
	return postTokenRequest(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     oauthClientID,
		"code":          code,
		"state":         state,
		"redirect_uri":  oauthRedirectURI,
		"code_verifier": verifier,
	})
}

// RefreshToken refreshes an expired OAuth token.
func RefreshToken(refreshToken string) (*OAuthCredentials, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("cannot refresh: empty refresh token")
	}
	creds, err := postTokenRequest(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     oauthClientID,
		"refresh_token": refreshToken,
	})
	if err != nil {
		log.Printf("[oauth] warning: refresh token exchange failed: %v", err)
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}
	return creds, nil
}

// ── Credential storage ──────────────────────────────────────────────────────

func loadCredentials() (*OAuthCredentials, error) {
	path := authDir()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials from %s: %w", path, err)
	}
	var creds OAuthCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials from %s: %w", path, err)
	}
	return &creds, nil
}

// saveCredentials persists OAuth credentials to disk (chmod 0600).
func saveCredentials(creds *OAuthCredentials) error {
	dir := filepath.Dir(authDir())
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Printf("[oauth] warning: could not create auth directory %s: %v", dir, err)
		return fmt.Errorf("failed to create auth directory: %w", err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}
	return os.WriteFile(authDir(), data, 0600)
}

// SaveCredentials is the public version for external callers.
func SaveCredentials(creds *OAuthCredentials) error {
	return saveCredentials(creds)
}
