package safety

import "regexp"

var secretPatterns = []struct {
	P *regexp.Regexp
	D string
}{
	{regexp.MustCompile(`(?i)(?:api[_\-]?key|apikey)\s*[=:]\s*["']?[A-Za-z0-9\-_]{16,}["']?`), "API key"},
	{regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`), "Secret key"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "AWS key"},
	{regexp.MustCompile(`(?i)(?:password|passwd|secret|token)\s*[=:]\s*["'][^"'${}]{6,}["']`), "Credential"},
	{regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`), "Private key"},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`), "GitHub PAT"},
}

// ScanSecrets checks content for hardcoded secrets.
func ScanSecrets(content string) (string, bool) {
	for _, s := range secretPatterns {
		if s.P.MatchString(content) {
			m := s.P.FindString(content)
			if len(m) > 12 {
				m = m[:12] + "..."
			}
			return s.D + ": " + m, true
		}
	}
	return "", false
}

var dangerPatterns = []struct {
	L string
	P *regexp.Regexp
}{
	{"rm-rf-root", regexp.MustCompile(`\brm\s+-[^\s]*r[^\s]*f[^\s]*\s+/`)},
	{"no-preserve-root", regexp.MustCompile(`\brm\b[^|&;]*--no-preserve-root`)},
	{"fork-bomb", regexp.MustCompile(`:\(\)\s*\{[^}]*:\s*\|\s*:`)},
	{"mkfs", regexp.MustCompile(`\bmkfs(?:\.[a-z0-9]+)?\s`)},
	{"sysrq", regexp.MustCompile(`/proc/sysrq-trigger`)},
}

// CheckSafety returns a label and true if the command is dangerous.
func CheckSafety(cmd string) (string, bool) {
	for _, d := range dangerPatterns {
		if d.P.MatchString(cmd) {
			return d.L, true
		}
	}
	return "", false
}
