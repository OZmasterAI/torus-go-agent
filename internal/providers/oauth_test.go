package providers

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ── Tests for PKCE helper functions ──

// TestGeneratePKCE tests that generatePKCE produces valid output.
func TestGeneratePKCE(t *testing.T) {
	verifier, challenge := generatePKCE()

	// Verifier should be 43 chars (32 bytes * 4/3 with base64url, no padding).
	if len(verifier) != 43 {
		t.Errorf("verifier length = %d, want 43", len(verifier))
	}

	// Challenge should be 43 chars (32 bytes hash * 4/3 with base64url, no padding).
	if len(challenge) != 43 {
		t.Errorf("challenge length = %d, want 43", len(challenge))
	}

	// Both should be non-empty.
	if verifier == "" {
		t.Fatal("verifier is empty")
	}
	if challenge == "" {
		t.Fatal("challenge is empty")
	}

	// Verifier and challenge should be different.
	if verifier == challenge {
		t.Fatal("verifier and challenge should not be equal")
	}

	// Challenge should be the SHA256 hash of the verifier.
	// Verify by computing the hash ourselves.
	hash := sha256.Sum256([]byte(verifier))
	expectedChallenge := base64URLEncode(hash[:])
	if challenge != expectedChallenge {
		t.Errorf("challenge mismatch: got %q, want %q", challenge, expectedChallenge)
	}
}

// TestGeneratePKCEUniqueness tests that generatePKCE produces unique values on each call.
func TestGeneratePKCEUniqueness(t *testing.T) {
	v1, c1 := generatePKCE()
	v2, c2 := generatePKCE()

	if v1 == v2 {
		t.Fatal("generatePKCE should produce unique verifiers")
	}
	if c1 == c2 {
		t.Fatal("generatePKCE should produce unique challenges")
	}
}

// TestBase64URLEncode tests URL-safe base64 encoding.
func TestBase64URLEncode(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte("hello"), "aGVsbG8"},
		{[]byte("test+/=="), "dGVzdCsvPT0"},  // base64url without padding
		{[]byte{0xFF, 0xFE, 0xFD}, "__79"},   // uses - and _ instead of + and /
	}

	for _, tt := range tests {
		got := base64URLEncode(tt.input)
		if got != tt.expected {
			t.Errorf("base64URLEncode(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ── Tests for HasAuth ──

// TestHasAuthWithEnvVar tests HasAuth returns true when ANTHROPIC_API_KEY is set.
func TestHasAuthWithEnvVar(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	if !HasAuth() {
		t.Fatal("HasAuth should return true when ANTHROPIC_API_KEY is set")
	}
}

// Note: TestHasAuthNoAuth would test absence of auth but requires file system state
// that is environment-dependent. The other HasAuth tests cover the env var path.

// Note: TestIsOAuthToken exists in anthropic_test.go and covers the IsOAuthToken function

// ── Tests for GetAnthropicKey ──

// TestGetAnthropicKeyFromEnv tests that GetAnthropicKey returns the env var if set.
func TestGetAnthropicKeyFromEnv(t *testing.T) {
	envKey := "test-api-key-from-env-12345"
	t.Setenv("ANTHROPIC_API_KEY", envKey)

	key, err := GetAnthropicKey()
	if err != nil {
		t.Fatalf("GetAnthropicKey failed: %v", err)
	}
	if key != envKey {
		t.Errorf("key = %q, want %q", key, envKey)
	}
}

// ── Tests for OAuthCredentials struct ──

// TestOAuthCredentialsJSON tests JSON marshaling/unmarshaling of OAuthCredentials.
func TestOAuthCredentialsJSON(t *testing.T) {
	original := &OAuthCredentials{
		Access:    "access-token-abc123",
		Refresh:   "refresh-token-xyz789",
		ExpiresAt: 1234567890000,
	}

	// Marshal to JSON.
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back.
	var recovered OAuthCredentials
	err = json.Unmarshal(data, &recovered)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify fields match.
	if recovered.Access != original.Access {
		t.Errorf("Access mismatch: got %q, want %q", recovered.Access, original.Access)
	}
	if recovered.Refresh != original.Refresh {
		t.Errorf("Refresh mismatch: got %q, want %q", recovered.Refresh, original.Refresh)
	}
	if recovered.ExpiresAt != original.ExpiresAt {
		t.Errorf("ExpiresAt mismatch: got %d, want %d", recovered.ExpiresAt, original.ExpiresAt)
	}
}

// TestOAuthCredentialsRoundTrip tests saving and loading credentials from disk.
// Since authDir() is not overridable, we test the JSON serialization behavior.
func TestOAuthCredentialsRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "auth.json")

	original := &OAuthCredentials{
		Access:    "test-access-token-12345",
		Refresh:   "test-refresh-token-67890",
		ExpiresAt: time.Now().UnixMilli() + 3600000, // 1 hour from now
	}

	// Manually marshal and write (simulating saveCredentials).
	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	err = os.WriteFile(credFile, data, 0600)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Verify file was created with correct permissions.
	fileInfo, err := os.Stat(credFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if fileInfo.Mode()&0o077 != 0 {
		t.Errorf("file permissions = %o, want 0600", fileInfo.Mode()&0o777)
	}

	// Manually read and unmarshal (simulating loadCredentials).
	readData, err := os.ReadFile(credFile)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	var recovered OAuthCredentials
	err = json.Unmarshal(readData, &recovered)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify the round-trip.
	if recovered.Access != original.Access {
		t.Errorf("Access mismatch: got %q, want %q", recovered.Access, original.Access)
	}
	if recovered.Refresh != original.Refresh {
		t.Errorf("Refresh mismatch: got %q, want %q", recovered.Refresh, original.Refresh)
	}
	if recovered.ExpiresAt != original.ExpiresAt {
		t.Errorf("ExpiresAt mismatch: got %d, want %d", recovered.ExpiresAt, original.ExpiresAt)
	}
}

// ── Benchmarks ──

// BenchmarkGeneratePKCE benchmarks the PKCE generation.
func BenchmarkGeneratePKCE(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = generatePKCE()
	}
}

// BenchmarkBase64URLEncode benchmarks the base64 encoding.
func BenchmarkBase64URLEncode(b *testing.B) {
	data := []byte("test data for benchmarking base64 encoding with PKCE")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = base64URLEncode(data)
	}
}
