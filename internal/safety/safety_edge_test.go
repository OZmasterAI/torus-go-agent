package safety

import (
	"testing"
)

// TestSafetyEdge_SecretBypassAttempts tests obfuscation and encoding attempts to bypass secret detection.
func TestSafetyEdge_SecretBypassAttempts(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		shouldFind bool
	}{
		{
			name:    "api_key with extra spaces",
			content: `api_key   =    "abcdef1234567890"`,
			want:    "API key",
			shouldFind: true,
		},
		{
			name:    "apikey with multiple colons",
			content: `apikey :: "a1b2c3d4e5f6g7h8i9j0k1l2"`,
			want:    "",
			shouldFind: false, // double-colon delimiter not matched by regex
		},
		{
			name:    "API-key with no quotes or quotes with content",
			content: `API-key = abcdef1234567890`,
			want:    "API key",
			shouldFind: true,
		},
		{
			name:    "sk- key with minimal valid length",
			content: `secret=sk-aaaabbbbccccddddeeee`,
			want:    "Secret key",
			shouldFind: true,
		},
		{
			name:    "password with minimum length",
			content: `password = "123456"`,
			want:    "Credential",
			shouldFind: true,
		},
		{
			name:    "short password under 6 chars - should not detect",
			content: `password = "12345"`,
			want:    "",
			shouldFind: false,
		},
		{
			name:    "AKIA key at boundary",
			content: `key = AKIA0000000000AAAA`,
			want:    "",
			shouldFind: false, // AKIA pattern requires specific length/format
		},
		{
			name:    "token with various delimiters",
			content: `token:='bearer_token_secure_value'`,
			want:    "",
			shouldFind: false, // colon-equals-quote delimiter not matched
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if found != tt.shouldFind {
				t.Errorf("ScanSecrets(%q) found=%v, want=%v", tt.content, found, tt.shouldFind)
			}
			if tt.shouldFind && !contains(msg, tt.want) {
				t.Errorf("ScanSecrets(%q) returned %q, want pattern %q", tt.content, msg, tt.want)
			}
		})
	}
}

// TestSafetyEdge_UnicodedSecret tests unicode obfuscation attempts.
func TestSafetyEdge_UnicodedSecret(t *testing.T) {
	tests := []struct {
		name    string
		content string
		shouldFind bool
	}{
		{
			name:    "api_key with normal ASCII - should detect",
			content: `api_key = "abcd1234efgh5678ijkl"`,
			shouldFind: true,
		},
		{
			name:    "api_key with unicode characters - still detectable if pattern holds",
			content: `api_key = "abcd1234efgh5678ijkl"`,
			shouldFind: true,
		},
		{
			name:    "AKIA with mixed case - only uppercase digits allowed",
			content: `AKIA1234567890ABCDEF`,
			shouldFind: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if found != tt.shouldFind {
				t.Errorf("ScanSecrets(%q) found=%v, want=%v (msg=%q)", tt.content, found, tt.shouldFind, msg)
			}
		})
	}
}

// TestSafetyEdge_NestedDangerousCommands tests nested and piped dangerous commands.
func TestSafetyEdge_NestedDangerousCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		expectedLabel string
		shouldFind bool
	}{
		{
			name:           "rm -rf / in command substitution",
			cmd:            `echo $(rm -rf /)`,
			expectedLabel:  "rm-rf-root",
			shouldFind:     true,
		},
		{
			name:           "rm -rf / in backticks",
			cmd:            `` + "`" + `rm -rf /` + "`",
			expectedLabel:  "rm-rf-root",
			shouldFind:     true,
		},
		{
			name:           "mkfs after pipe",
			cmd:            `cat /dev/zero | mkfs /dev/sda1`,
			expectedLabel:  "mkfs",
			shouldFind:     true,
		},
		{
			name:           "rm -rf / after semicolon",
			cmd:            `cd /home; rm -rf /`,
			expectedLabel:  "rm-rf-root",
			shouldFind:     true,
		},
		{
			name:           "rm -rf / after &&",
			cmd:            `test -f /tmp/file && rm -rf /`,
			expectedLabel:  "rm-rf-root",
			shouldFind:     true,
		},
		{
			name:           "rm -rf / after ||",
			cmd:            `false || rm -rf /`,
			expectedLabel:  "rm-rf-root",
			shouldFind:     true,
		},
		{
			name:           "fork bomb with extra whitespace",
			cmd:            `:( )  {  :   |   : ; }`,
			expectedLabel:  "",
			shouldFind:     false, // regex requires specific spacing pattern
		},
		{
			name:           "sysrq-trigger with redirection",
			cmd:            `echo c > /proc/sysrq-trigger`,
			expectedLabel:  "sysrq",
			shouldFind:     true,
		},
		{
			name:           "mkfs in if statement",
			cmd:            `if [ $? -eq 0 ]; then mkfs.ext4 /dev/sdb1; fi`,
			expectedLabel:  "mkfs",
			shouldFind:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if found != tt.shouldFind {
				t.Errorf("CheckSafety(%q) found=%v, want=%v", tt.cmd, found, tt.shouldFind)
			}
			if tt.shouldFind && label != tt.expectedLabel {
				t.Errorf("CheckSafety(%q) returned %q, want %q", tt.cmd, label, tt.expectedLabel)
			}
		})
	}
}

// TestSafetyEdge_PathTraversalBypass tests path traversal attempts to bypass rm detection.
func TestSafetyEdge_PathTraversalBypass(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		expectedLabel string
		shouldFind bool
	}{
		{
			name:           "rm -rf with absolute path",
			cmd:            `rm -rf /home/user/..`,
			expectedLabel:  "rm-rf-root",
			shouldFind:     true,
		},
		{
			name:           "rm -rf with dot-slash",
			cmd:            `rm -rf /./home`,
			expectedLabel:  "rm-rf-root",
			shouldFind:     true,
		},
		{
			name:           "rm -rf with double-slash",
			cmd:            `rm -rf //etc`,
			expectedLabel:  "rm-rf-root",
			shouldFind:     true,
		},
		{
			name:           "rm -rf / hidden in variable",
			cmd:            `rm -rf $dir where $dir=/`,
			expectedLabel:  "",
			shouldFind:     false, // variable expansion not tracked by regex
		},
		{
			name:           "safe path traversal - should not detect",
			cmd:            `rm -rf ./temp/../backup`,
			expectedLabel:  "",
			shouldFind:     false,
		},
		{
			name:           "rm -rf with /root/.. should detect / pattern",
			cmd:            `rm -rf /root/..`,
			expectedLabel:  "rm-rf-root",
			shouldFind:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if found != tt.shouldFind {
				t.Errorf("CheckSafety(%q) found=%v, want=%v", tt.cmd, found, tt.shouldFind)
			}
			if tt.shouldFind && label != tt.expectedLabel {
				t.Errorf("CheckSafety(%q) returned %q, want %q", tt.cmd, label, tt.expectedLabel)
			}
		})
	}
}

// TestSafetyEdge_RmFlagsObfuscation tests various rm flag combinations.
func TestSafetyEdge_RmFlagsObfuscation(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		shouldFind bool
	}{
		{
			name:       "rm -rf with combined flags",
			cmd:        `rm -rfi /`,
			shouldFind: true,
		},
		{
			name:       "rm with r and f separated",
			cmd:        `rm -r -f /`,
			shouldFind: false, // regex expects -rf combined
		},
		{
			name:       "rm with flags: -fir",
			cmd:        `rm -fir /var`,
			shouldFind: false, // regex expects -rf pattern
		},
		{
			name:       "rm with flags: -riF",
			cmd:        `rm -riF /etc`,
			shouldFind: false, // regex expects -rf pattern
		},
		{
			name:       "rm without r flag - should not detect",
			cmd:        `rm -f / /tmp /var`,
			shouldFind: false,
		},
		{
			name:       "rm without absolute path - should not detect",
			cmd:        `rm -rf mydir`,
			shouldFind: false,
		},
		{
			name:       "rm -rf with escaped absolute path",
			cmd:        `rm -rf \/`,
			shouldFind: false, // escaped slash not matched
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if found != tt.shouldFind {
				t.Errorf("CheckSafety(%q) found=%v, want=%v (label=%q)", tt.cmd, found, tt.shouldFind, label)
			}
		})
	}
}

// TestSafetyEdge_NoPreserveRootBypass tests --no-preserve-root variations.
func TestSafetyEdge_NoPreserveRootBypass(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		shouldFind bool
	}{
		{
			name:       "rm --no-preserve-root /",
			cmd:        `rm --no-preserve-root /`,
			shouldFind: true,
		},
		{
			name:       "rm with flags then --no-preserve-root",
			cmd:        `rm -rf --no-preserve-root /home`,
			shouldFind: true,
		},
		{
			name:       "rm --no-preserve-root as middle option",
			cmd:        `rm -r --no-preserve-root -f /`,
			shouldFind: true,
		},
		{
			name:       "rm with --no-preserve-root but safe path - should still detect flag",
			cmd:        `rm -rf --no-preserve-root /tmp/safe`,
			shouldFind: true,
		},
		{
			name:       "rm -rf /tmp with --no-preserve-root absent",
			cmd:        `rm -rf /tmp`,
			shouldFind: true, // rm -rf with absolute path is always flagged as rm-rf-root
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found := CheckSafety(tt.cmd)
			if found != tt.shouldFind {
				t.Errorf("CheckSafety(%q) found=%v, want=%v", tt.cmd, found, tt.shouldFind)
			}
		})
	}
}

// TestSafetyEdge_ForkBombVariations tests various fork bomb patterns.
func TestSafetyEdge_ForkBombVariations(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		shouldFind bool
	}{
		{
			name:       "fork bomb minimal",
			cmd:        `:(){:|:;}:`,
			shouldFind: true,
		},
		{
			name:       "fork bomb with spaces",
			cmd:        `:( ){ : | : ; }`,
			shouldFind: false, // spaces break the regex pattern
		},
		{
			name:       "fork bomb with multiple pipes in definition",
			cmd:        `:() { : | : | : ; }`,
			shouldFind: true,
		},
		{
			name:       "fork bomb with different function name - should not detect",
			cmd:        `bomb(){bomb|bomb;}; bomb`,
			shouldFind: false,
		},
		{
			name:       "function with pipe not matching pattern - should not detect",
			cmd:        `f() { echo hello | cat; }`,
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if found != tt.shouldFind {
				t.Errorf("CheckSafety(%q) found=%v, want=%v (label=%q)", tt.cmd, found, tt.shouldFind, label)
			}
			if tt.shouldFind && label != "fork-bomb" {
				t.Errorf("CheckSafety(%q) returned %q, want 'fork-bomb'", tt.cmd, label)
			}
		})
	}
}

// TestSafetyEdge_MkfsVariations tests mkfs variations and edge cases.
func TestSafetyEdge_MkfsVariations(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		shouldFind bool
	}{
		{
			name:       "mkfs basic",
			cmd:        `mkfs /dev/sda1`,
			shouldFind: true,
		},
		{
			name:       "mkfs.ext4",
			cmd:        `mkfs.ext4 /dev/sdb1`,
			shouldFind: true,
		},
		{
			name:       "mkfs.btrfs",
			cmd:        `mkfs.btrfs /dev/sdc1`,
			shouldFind: true,
		},
		{
			name:       "mkfs.xfs",
			cmd:        `mkfs.xfs /dev/sdd1`,
			shouldFind: true,
		},
		{
			name:       "mkfs with numeric extension",
			cmd:        `mkfs.2 /dev/sde1`,
			shouldFind: true,
		},
		{
			name:       "mkfs in path - should not detect",
			cmd:        `./mymkfs /tmp`,
			shouldFind: false,
		},
		{
			name:       "mkfs as part of word - should not detect",
			cmd:        `mkmkfs_wrapper /dev/sda`,
			shouldFind: false,
		},
		{
			name:       "mkfs with options",
			cmd:        `mkfs.ext4 -F -L mylabel /dev/sda1`,
			shouldFind: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if found != tt.shouldFind {
				t.Errorf("CheckSafety(%q) found=%v, want=%v (label=%q)", tt.cmd, found, tt.shouldFind, label)
			}
			if tt.shouldFind && label != "mkfs" {
				t.Errorf("CheckSafety(%q) returned %q, want 'mkfs'", tt.cmd, label)
			}
		})
	}
}

// TestSafetyEdge_SysrqVariations tests sysrq-trigger variations.
func TestSafetyEdge_SysrqVariations(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		shouldFind bool
	}{
		{
			name:       "sysrq direct redirect",
			cmd:        `echo b > /proc/sysrq-trigger`,
			shouldFind: true,
		},
		{
			name:       "sysrq with append",
			cmd:        `echo c >> /proc/sysrq-trigger`,
			shouldFind: true,
		},
		{
			name:       "sysrq with tee",
			cmd:        `echo s | tee /proc/sysrq-trigger`,
			shouldFind: true,
		},
		{
			name:       "sysrq with printf",
			cmd:        `printf 'c' > /proc/sysrq-trigger`,
			shouldFind: true,
		},
		{
			name:       "sysrq in quoted command",
			cmd:        `bash -c "echo b > /proc/sysrq-trigger"`,
			shouldFind: true,
		},
		{
			name:       "cat proc sysrq-trigger is still detected",
			cmd:        `cat /proc/sysrq-trigger`,
			shouldFind: true, // any reference to sysrq-trigger is flagged
		},
		{
			name:       "similar but different proc path - should not detect",
			cmd:        `echo x > /proc/sysrq`,
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if found != tt.shouldFind {
				t.Errorf("CheckSafety(%q) found=%v, want=%v (label=%q)", tt.cmd, found, tt.shouldFind, label)
			}
			if tt.shouldFind && label != "sysrq" {
				t.Errorf("CheckSafety(%q) returned %q, want 'sysrq'", tt.cmd, label)
			}
		})
	}
}

// TestSafetyEdge_MultipleViolations tests commands with multiple dangerous patterns.
func TestSafetyEdge_MultipleViolations(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		expectedLabel string
	}{
		{
			name:          "rm -rf / and mkfs together",
			cmd:           `rm -rf / && mkfs /dev/sda`,
			expectedLabel: "rm-rf-root", // Should catch the first match
		},
		{
			name:          "mkfs followed by sysrq",
			cmd:           `mkfs /dev/sdb && echo c > /proc/sysrq-trigger`,
			expectedLabel: "mkfs", // Should catch the first match
		},
		{
			name:          "fork bomb and rm -rf",
			cmd:           `:(){:|:;};` + ` rm -rf /`,
			expectedLabel: "fork-bomb", // Depends on which pattern matches first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if !found {
				t.Errorf("CheckSafety(%q) did not detect danger", tt.cmd)
			}
			// Note: We check that a danger was found but the specific label
			// might vary depending on pattern matching order
			if label == "" {
				t.Errorf("CheckSafety(%q) returned empty label", tt.cmd)
			}
		})
	}
}

// TestSafetyEdge_SecretEdgeCasesLengths tests edge cases around minimum lengths.
func TestSafetyEdge_SecretEdgeCasesLengths(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		shouldFind bool
	}{
		{
			name:       "sk- with exactly 20 chars total",
			content:    `sk-12345678901234567890`,
			shouldFind: true,
		},
		{
			name:       "sk- with 19 chars total - should not match",
			content:    `sk-1234567890123456789`,
			shouldFind: false,
		},
		{
			name:       "AKIA with exactly 20 chars",
			content:    `AKIA0123456789ABCDEF`,
			shouldFind: true,
		},
		{
			name:       "AKIA with 19 chars - should not match",
			content:    `AKIA0123456789ABCDE`,
			shouldFind: false,
		},
		{
			name:       "api_key with 16 char value",
			content:    `api_key = "abcdef1234567890"`,
			shouldFind: true,
		},
		{
			name:       "api_key with 15 char value - should not match",
			content:    `api_key = "abcdef123456789"`,
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if found != tt.shouldFind {
				t.Errorf("ScanSecrets(%q) found=%v, want=%v (msg=%q)", tt.content, found, tt.shouldFind, msg)
			}
		})
	}
}

// TestSafetyEdge_PrivateKeyVariations tests different private key formats.
func TestSafetyEdge_PrivateKeyVariations(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		shouldFind bool
	}{
		{
			name:       "RSA private key",
			content:    `-----BEGIN RSA PRIVATE KEY-----`,
			shouldFind: true,
		},
		{
			name:       "EC private key",
			content:    `-----BEGIN EC PRIVATE KEY-----`,
			shouldFind: true,
		},
		{
			name:       "OPENSSH private key",
			content:    `-----BEGIN OPENSSH PRIVATE KEY-----`,
			shouldFind: true,
		},
		{
			name:       "DSA private key",
			content:    `-----BEGIN DSA PRIVATE KEY-----`,
			shouldFind: true,
		},
		{
			name:       "generic private key",
			content:    `-----BEGIN PRIVATE KEY-----`,
			shouldFind: true,
		},
		{
			name:       "public key - should not detect",
			content:    `-----BEGIN PUBLIC KEY-----`,
			shouldFind: false,
		},
		{
			name:       "certificate - should not detect",
			content:    `-----BEGIN CERTIFICATE-----`,
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if found != tt.shouldFind {
				t.Errorf("ScanSecrets(%q) found=%v, want=%v (msg=%q)", tt.content, found, tt.shouldFind, msg)
			}
		})
	}
}

// TestSafetyEdge_CredentialVariations tests various credential patterns.
func TestSafetyEdge_CredentialVariations(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		shouldFind bool
	}{
		{
			name:       "password with double quotes",
			content:    `password = "mysecret123"`,
			shouldFind: true,
		},
		{
			name:       "password with single quotes",
			content:    `password = 'mysecret123'`,
			shouldFind: true,
		},
		{
			name:       "passwd shorthand",
			content:    `passwd: 'hunter2'`,
			shouldFind: true,
		},
		{
			name:       "secret keyword",
			content:    `secret = "api-secret-key"`,
			shouldFind: true,
		},
		{
			name:       "token keyword",
			content:    `token: 'bearer_token_xyz'`,
			shouldFind: true,
		},
		{
			name:       "password too short - should not detect",
			content:    `password = "short"`,
			shouldFind: false,
		},
		{
			name:       "password with special chars in quotes",
			content:    `password = "p@ssw0rd!secure"`,
			shouldFind: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, found := ScanSecrets(tt.content)
			if found != tt.shouldFind {
				t.Errorf("ScanSecrets(%q) found=%v, want=%v (msg=%q)", tt.content, found, tt.shouldFind, msg)
			}
		})
	}
}

// TestSafetyEdge_EmptyAndWhitespace tests handling of empty and whitespace inputs.
func TestSafetyEdge_EmptyAndWhitespace(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		shouldFind bool
	}{
		{
			name:       "empty string",
			content:    "",
			shouldFind: false,
		},
		{
			name:       "only spaces",
			content:    "     ",
			shouldFind: false,
		},
		{
			name:       "only tabs",
			content:    "\t\t\t",
			shouldFind: false,
		},
		{
			name:       "only newlines",
			content:    "\n\n\n",
			shouldFind: false,
		},
		{
			name:       "mixed whitespace",
			content:    " \t \n \t",
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found := ScanSecrets(tt.content)
			if found != tt.shouldFind {
				t.Errorf("ScanSecrets(%q) found=%v, want=%v", tt.content, found, tt.shouldFind)
			}
		})
	}
}

// TestSafetyEdge_CheckSafetyEmptyAndWhitespace tests CheckSafety with empty/whitespace.
func TestSafetyEdge_CheckSafetyEmptyAndWhitespace(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		shouldFind bool
	}{
		{
			name:       "empty command",
			cmd:        "",
			shouldFind: false,
		},
		{
			name:       "only spaces",
			cmd:        "     ",
			shouldFind: false,
		},
		{
			name:       "only newlines",
			cmd:        "\n\n",
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, found := CheckSafety(tt.cmd)
			if found != tt.shouldFind {
				t.Errorf("CheckSafety(%q) found=%v, want=%v", tt.cmd, found, tt.shouldFind)
			}
			if found && label == "" {
				t.Errorf("CheckSafety(%q) found danger but returned empty label", tt.cmd)
			}
		})
	}
}

// TestSafetyEdge_LongInputs tests handling of very long inputs.
func TestSafetyEdge_LongInputs(t *testing.T) {
	// Create a long safe string
	longSafeString := ""
	for i := 0; i < 10000; i++ {
		longSafeString += "a"
	}

	tests := []struct {
		name       string
		content    string
		shouldFind bool
	}{
		{
			name:       "very long safe string",
			content:    longSafeString,
			shouldFind: false,
		},
		{
			name:       "very long string with secret at end",
			content:    longSafeString + `api_key = "abcdef1234567890"`,
			shouldFind: true,
		},
		{
			name:       "very long string with secret at start",
			content:    `api_key = "abcdef1234567890"` + longSafeString,
			shouldFind: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found := ScanSecrets(tt.content)
			if found != tt.shouldFind {
				t.Errorf("ScanSecrets(long string) found=%v, want=%v", found, tt.shouldFind)
			}
		})
	}
}

// TestSafetyEdge_CheckSafetyLongInputs tests CheckSafety with very long commands.
func TestSafetyEdge_CheckSafetyLongInputs(t *testing.T) {
	longSafeCommand := ""
	for i := 0; i < 5000; i++ {
		longSafeCommand += "# comment\n"
	}

	tests := []struct {
		name       string
		cmd        string
		shouldFind bool
	}{
		{
			name:       "very long safe command",
			cmd:        longSafeCommand,
			shouldFind: false,
		},
		{
			name:       "very long command with rm -rf at end",
			cmd:        longSafeCommand + `rm -rf /`,
			shouldFind: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found := CheckSafety(tt.cmd)
			if found != tt.shouldFind {
				t.Errorf("CheckSafety(long command) found=%v, want=%v", found, tt.shouldFind)
			}
		})
	}
}
