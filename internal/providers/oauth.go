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
	oauthClientID    = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	oauthAuthorizeURL = "https://claude.ai/oauth/authorize"
	oauthTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	oauthRedirectURI  = "https://console.anthropic.com/oauth/code/callback"
	oauthScopes       = "org:create_api_key user:profile user:inference"
)

// OAuthCredentials holds the OAuth tokens.
type OAuthCredentials struct {
	Access    string `json:"access"`
	Refresh   string `json:"refresh"`
	ExpiresAt int64  `json:"expires_at"` // unix ms
}

// authDir returns the credential storage directory.
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
// Priority: env var → OAuth token (auto-refreshed if expired).
func GetAnthropicKey() (string, error) {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key, nil
	}
	creds, err := loadCredentials()
	if err != nil {
		return "", fmt.Errorf("no API key and no OAuth credentials: %w", err)
	}
	// Auto-refresh if expired (5 min buffer)
	if time.Now().UnixMilli() >= creds.ExpiresAt-5*60*1000 {
		refreshed, err := RefreshToken(creds.Refresh)
		if err != nil {
			return "", fmt.Errorf("token refresh failed: %w", err)
		}
		creds = refreshed
		if err := saveCredentials(creds); err != nil {
			log.Printf("[oauth] warning: could not persist refreshed credentials: %v", err)
		}
	}
	return creds.Access, nil
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
	rand.Read(b)
	verifier = base64URLEncode(b)
	hash := sha256.Sum256([]byte(verifier))
	challenge = base64URLEncode(hash[:])
	return
}

// ── Login flow ──────────────────────────────────────────────────────────────

// LoginAnthropic runs the OAuth PKCE flow.
// onAuthURL is called with the URL to open in browser.
// onPromptCode is called to get the pasted "code#state" from the user.
func LoginAnthropic(onAuthURL func(string), onPromptCode func() (string, error)) (*OAuthCredentials, error) {
	verifier, challenge := generatePKCE()

	params := url.Values{
		"code":                  {"true"},
		"client_id":             {oauthClientID},
		"response_type":         {"code"},
		"redirect_uri":          {oauthRedirectURI},
		"scope":                 {oauthScopes},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {verifier},
	}

	authURL := oauthAuthorizeURL + "?" + params.Encode()
	onAuthURL(authURL)

	codeState, err := onPromptCode()
	if err != nil {
		return nil, fmt.Errorf("prompt code: %w", err)
	}

	parts := strings.SplitN(codeState, "#", 2)
	code := parts[0]
	state := ""
	if len(parts) > 1 {
		state = parts[1]
	}

	return exchangeCode(code, state, verifier)
}

func exchangeCode(code, state, verifier string) (*OAuthCredentials, error) {
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     oauthClientID,
		"code":          code,
		"state":         state,
		"redirect_uri":  oauthRedirectURI,
		"code_verifier": verifier,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(oauthTokenURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var errBody map[string]any
		json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("token exchange failed (%d): %v", resp.StatusCode, errBody)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)

	return &OAuthCredentials{
		Access:    tokenResp.AccessToken,
		Refresh:   tokenResp.RefreshToken,
		ExpiresAt: time.Now().UnixMilli() + tokenResp.ExpiresIn*1000 - 5*60*1000,
	}, nil
}

// RefreshToken refreshes an expired OAuth token.
func RefreshToken(refreshToken string) (*OAuthCredentials, error) {
	payload := map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     oauthClientID,
		"refresh_token": refreshToken,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(oauthTokenURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("refresh: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("refresh failed (%d)", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)

	return &OAuthCredentials{
		Access:    tokenResp.AccessToken,
		Refresh:   tokenResp.RefreshToken,
		ExpiresAt: time.Now().UnixMilli() + tokenResp.ExpiresIn*1000 - 5*60*1000,
	}, nil
}

// ── Credential storage ──────────────────────────────────────────────────────

func loadCredentials() (*OAuthCredentials, error) {
	data, err := os.ReadFile(authDir())
	if err != nil {
		return nil, err
	}
	var creds OAuthCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// SaveCredentials persists OAuth credentials to disk (chmod 0600).
func saveCredentials(creds *OAuthCredentials) error {
	dir := filepath.Dir(authDir())
	os.MkdirAll(dir, 0700)
	data, _ := json.MarshalIndent(creds, "", "  ")
	return os.WriteFile(authDir(), data, 0600)
}

// SaveCredentials is the public version for external callers.
func SaveCredentials(creds *OAuthCredentials) error {
	return saveCredentials(creds)
}
