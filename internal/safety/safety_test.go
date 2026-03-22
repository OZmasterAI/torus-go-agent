package safety

import (
	"testing"
)

// TestScanSecrets_CleanText tests that clean text passes without detection.
func TestScanSecrets_CleanText(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "empty string",
			content: "",
		},
		{
			name:    "normal text",
			content: "This is a normal text without any secrets",
		},
		{
			name:    "variable names",
			content: "var api_key = someFunction()",
		},
		{
			name:    "documentation",
			content: "To use the API key, set it in your environment",
		},
		{
			name:    "code comment",
			content: "// apiKey is the user's authentication token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if found {
				t.Errorf("ScanSecrets(%q) found secret unexpectedly: %s", tt.content, msg)
			}
			if msg != "" {
				t.Errorf("ScanSecrets(%q) returned non-empty message: %s", tt.content, msg)
			}
		})
	}
}

// TestScanSecrets_APIKey tests detection of API key patterns.
func TestScanSecrets_APIKey(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "API key with equals and quotes",
			content: `api_key = "abcdef1234567890"`,
			want:    "API key",
		},
		{
			name:    "apikey no underscore",
			content: `apikey: "a1b2c3d4e5f6g7h8i9j0k1l2"`,
			want:    "API key",
		},
		{
			name:    "API-key with dash",
			content: `API-key="z9y8x7w6v5u4t3s2r1q0p9o8"`,
			want:    "API key",
		},
		{
			name:    "APIKEY uppercase",
			content: `APIKEY = "secrettoken1234567890abcd"`,
			want:    "API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if !found {
				t.Errorf("ScanSecrets(%q) did not find secret", tt.content)
			}
			if !matchesPattern(msg, tt.want) {
				t.Errorf("ScanSecrets(%q) returned %q, want pattern %q", tt.content, msg, tt.want)
			}
		})
	}
}

// TestScanSecrets_AWSKey tests detection of AWS key patterns.
func TestScanSecrets_AWSKey(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "AWS key in code",
			content: `accessKey := "AKIAIOSFODNN7EXAMPLE"`,
			want:    "AWS key",
		},
		{
			name:    "AWS key in credentials",
			content: `export AWS_KEY="AKIA2EXAMPLE1234567890"`,
			want:    "AWS key",
		},
		{
			name:    "AWS key pattern AKIA + 16 chars uppercase/digits",
			content: `aws_key = AKIA1234567890ABCDEF`,
			want:    "AWS key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if !found {
				t.Errorf("ScanSecrets(%q) did not find secret", tt.content)
			}
			if !matchesPattern(msg, tt.want) {
				t.Errorf("ScanSecrets(%q) returned %q, want pattern %q", tt.content, msg, tt.want)
			}
		})
	}
}

// TestScanSecrets_SecretKey tests detection of secret key patterns (e.g., OpenAI sk-* keys).
func TestScanSecrets_SecretKey(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "sk- prefix 20+ chars",
			content: `secret="sk-abcdefghij1234567890"`,
			want:    "Secret key",
		},
		{
			name:    "sk- with alphanumerics",
			content: `key=sk-proj1234567890abcdefghij`,
			want:    "Secret key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if !found {
				t.Errorf("ScanSecrets(%q) did not find secret", tt.content)
			}
			if !matchesPattern(msg, tt.want) {
				t.Errorf("ScanSecrets(%q) returned %q, want pattern %q", tt.content, msg, tt.want)
			}
		})
	}
}

// TestScanSecrets_Credential tests detection of password/token/secret patterns.
func TestScanSecrets_Credential(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "password in quotes",
			content: `password = "mysecretpassword123"`,
			want:    "Credential",
		},
		{
			name:    "passwd shorthand",
			content: `passwd: 'hunter2secure'`,
			want:    "Credential",
		},
		{
			name:    "token in env",
			content: `export TOKEN="bearer_token_xyz789"`,
			want:    "Credential",
		},
		{
			name:    "secret in yaml",
			content: `secret: 'app-secret-key-value'`,
			want:    "Credential",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if !found {
				t.Errorf("ScanSecrets(%q) did not find secret", tt.content)
			}
			if !matchesPattern(msg, tt.want) {
				t.Errorf("ScanSecrets(%q) returned %q, want pattern %q", tt.content, msg, tt.want)
			}
		})
	}
}

// TestScanSecrets_PrivateKey tests detection of private key PEM headers.
func TestScanSecrets_PrivateKey(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "RSA private key",
			content: `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA1234567890abcdefg...
-----END RSA PRIVATE KEY-----`,
			want: "Private key",
		},
		{
			name:    "EC private key",
			content: `-----BEGIN EC PRIVATE KEY-----`,
			want:    "Private key",
		},
		{
			name:    "OPENSSH private key",
			content: `-----BEGIN OPENSSH PRIVATE KEY-----`,
			want:    "Private key",
		},
		{
			name:    "DSA private key",
			content: `-----BEGIN DSA PRIVATE KEY-----`,
			want:    "Private key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if !found {
				t.Errorf("ScanSecrets(%q) did not find secret", tt.content)
			}
			if !matchesPattern(msg, tt.want) {
				t.Errorf("ScanSecrets(%q) returned %q, want pattern %q", tt.content, msg, tt.want)
			}
		})
	}
}

// TestScanSecrets_GitHubPAT tests detection of GitHub Personal Access Token patterns.
func TestScanSecrets_GitHubPAT(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "GitHub PAT token",
			content: `github_pat = "ghp_1234567890ABCDEF1234567890ABCDEF1234"`,
			want:    "GitHub PAT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if !found {
				t.Errorf("ScanSecrets(%q) did not find secret", tt.content)
			}
			if !matchesPattern(msg, tt.want) {
				t.Errorf("ScanSecrets(%q) returned %q, want pattern %q", tt.content, msg, tt.want)
			}
		})
	}
}

// TestScanSecrets_Truncation tests that long matches are truncated to 12 chars + "...".
func TestScanSecrets_Truncation(t *testing.T) {
	content := `aws_key = "AKIA0123456789ABCDEFGHIJKLMNOPQRSTUV"`
	msg, found := ScanSecrets(content)
	if !found {
		t.Errorf("ScanSecrets(%q) did not find secret", content)
	}

	// Message should contain "..." indicating truncation
	if !contains(msg, "...") {
		t.Errorf("ScanSecrets(%q) returned %q, expected truncation with '...'", content, msg)
	}

	// Check that the match part (after ": ") is 15 chars or less (12 + "...")
	parts := split(msg, ": ")
	if len(parts) > 1 {
		matchPart := parts[1]
		if len(matchPart) > 15 { // 12 + 3 for "..."
			t.Errorf("ScanSecrets truncation produced %q, expected max 15 chars", matchPart)
		}
	}
}

// TestCheckSafety_SafeInput tests that normal commands pass safety checks.
func TestCheckSafety_SafeInput(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{
			name: "ls command",
			cmd:  "ls -la /home",
		},
		{
			name: "grep search",
			cmd:  "grep -r 'pattern' /home/user",
		},
		{
			name: "mkdir command",
			cmd:  "mkdir -p /home/user/documents",
		},
		{
			name: "cd command",
			cmd:  "cd /home && ls -la",
		},
		{
			name: "mv safe",
			cmd:  "mv file.txt archive/",
		},
		{
			name: "cp safe",
			cmd:  "cp -r documents backup",
		},
		{
			name: "echo safe",
			cmd:  "echo 'Hello World'",
		},
		{
			name: "find safe",
			cmd:  "find /home -name '*.txt'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if found {
				t.Errorf("CheckSafety(%q) marked as dangerous unexpectedly: %s", tt.cmd, label)
			}
			if label != "" {
				t.Errorf("CheckSafety(%q) returned non-empty label: %s", tt.cmd, label)
			}
		})
	}
}

// TestCheckSafety_RmRfRoot tests detection of rm -rf / patterns.
func TestCheckSafety_RmRfRoot(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{
			name: "rm -rf /",
			cmd:  "rm -rf /",
		},
		{
			name: "rm -rf /root",
			cmd:  "rm -rf /root",
		},
		{
			name: "rm -rfi /var",
			cmd:  "rm -rfi /var",
		},
		{
			name: "rm -rXf /home",
			cmd:  "rm -rif /home",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if !found {
				t.Errorf("CheckSafety(%q) did not detect danger", tt.cmd)
			}
			if label != "rm-rf-root" {
				t.Errorf("CheckSafety(%q) returned %q, want 'rm-rf-root'", tt.cmd, label)
			}
		})
	}
}

// TestCheckSafety_NoPreserveRoot tests detection of rm --no-preserve-root patterns.
func TestCheckSafety_NoPreserveRoot(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{
			name: "rm --no-preserve-root",
			cmd:  "rm --no-preserve-root /",
		},
		{
			name: "rm -rf --no-preserve-root /",
			cmd:  "rm -rf --no-preserve-root /",
		},
		{
			name: "rm -r --no-preserve-root -f /home",
			cmd:  "rm -r --no-preserve-root -f /home",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if !found {
				t.Errorf("CheckSafety(%q) did not detect danger", tt.cmd)
			}
			if label != "no-preserve-root" {
				t.Errorf("CheckSafety(%q) returned %q, want 'no-preserve-root'", tt.cmd, label)
			}
		})
	}
}

// TestCheckSafety_ForkBomb tests detection of fork bomb patterns.
func TestCheckSafety_ForkBomb(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{
			name: "basic fork bomb",
			cmd:  `:() { : | : ; }`,
		},
		{
			name: "fork bomb variant",
			cmd:  `:(){:|:;}:`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if !found {
				t.Errorf("CheckSafety(%q) did not detect danger", tt.cmd)
			}
			if label != "fork-bomb" {
				t.Errorf("CheckSafety(%q) returned %q, want 'fork-bomb'", tt.cmd, label)
			}
		})
	}
}

// TestCheckSafety_Mkfs tests detection of mkfs patterns.
func TestCheckSafety_Mkfs(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{
			name: "mkfs basic",
			cmd:  "mkfs /dev/sda1",
		},
		{
			name: "mkfs.ext4",
			cmd:  "mkfs.ext4 /dev/sdb1",
		},
		{
			name: "mkfs.ntfs",
			cmd:  "mkfs.ntfs /dev/sdc1",
		},
		{
			name: "mkfs with flag",
			cmd:  "mkfs.ext3 -L mypart /dev/sdd1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if !found {
				t.Errorf("CheckSafety(%q) did not detect danger", tt.cmd)
			}
			if label != "mkfs" {
				t.Errorf("CheckSafety(%q) returned %q, want 'mkfs'", tt.cmd, label)
			}
		})
	}
}

// TestCheckSafety_Sysrq tests detection of sysrq-trigger patterns.
func TestCheckSafety_Sysrq(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{
			name: "sysrq direct",
			cmd:  "echo b > /proc/sysrq-trigger",
		},
		{
			name: "sysrq in command",
			cmd:  "printf 'c' | tee /proc/sysrq-trigger",
		},
		{
			name: "sysrq with cat",
			cmd:  "cat something > /proc/sysrq-trigger",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if !found {
				t.Errorf("CheckSafety(%q) did not detect danger", tt.cmd)
			}
			if label != "sysrq" {
				t.Errorf("CheckSafety(%q) returned %q, want 'sysrq'", tt.cmd, label)
			}
		})
	}
}

// TestCheckSafety_BlockedInput tests that various dangerous inputs are blocked.
func TestCheckSafety_BlockedInput(t *testing.T) {
	tests := []struct {
		name          string
		cmd           string
		expectedLabel string
	}{
		{
			name:          "rm -rf root",
			cmd:           "rm -rf /",
			expectedLabel: "rm-rf-root",
		},
		{
			name:          "rm with no preserve root",
			cmd:           "rm --no-preserve-root -rf /",
			expectedLabel: "no-preserve-root",
		},
		{
			name:          "fork bomb",
			cmd:           `:(){:|:;}:`,
			expectedLabel: "fork-bomb",
		},
		{
			name:          "mkfs filesystem",
			cmd:           "mkfs.ext4 /dev/sda1",
			expectedLabel: "mkfs",
		},
		{
			name:          "sysrq trigger",
			cmd:           "echo c > /proc/sysrq-trigger",
			expectedLabel: "sysrq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if !found {
				t.Errorf("CheckSafety(%q) should have been blocked", tt.cmd)
			}
			if label != tt.expectedLabel {
				t.Errorf("CheckSafety(%q) returned %q, want %q", tt.cmd, label, tt.expectedLabel)
			}
		})
	}
}

// Helper function to check if message contains expected pattern
func matchesPattern(msg, pattern string) bool {
	return contains(msg, pattern)
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper function to split string (simple version)
func split(s, sep string) []string {
	if len(sep) == 0 {
		return []string{s}
	}

	parts := make([]string, 0, 2)
	idx := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			parts = append(parts, s[idx:i])
			idx = i + len(sep)
		}
	}
	parts = append(parts, s[idx:])
	return parts
}
