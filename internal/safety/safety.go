package safety

import (
	"regexp"
	"strings"
)

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

// cmdSep splits a command line into individual command segments so that
// flags and targets from separate commands are not mixed together.
var cmdSep = regexp.MustCompile(`[;|&\n]+`)

// isRootTarget reports whether an rm target resolves to a catastrophic path
// (root, home, or a top-level glob).
func isRootTarget(t string) bool {
	switch t {
	case "/", "~", "*", "/*", "~/", "~/*", "/.":
		return true
	}
	return false
}

// isDangerousRm parses each command segment for an rm invocation and reports
// true when the recursive (-r/-R/--recursive) and force (-f/--force) flags are
// both present, in any order or combined form (e.g. -rf, -fr, -r -f), and one
// of the targets resolves to a catastrophic path. This makes detection
// order-independent, unlike a single positional regex.
func isDangerousRm(cmd string) bool {
	for _, seg := range cmdSep.Split(cmd, -1) {
		fields := strings.Fields(seg)
		idx := -1
		for i, f := range fields {
			if f == "rm" {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		recursive := false
		force := false
		rootTarget := false
		for _, f := range fields[idx+1:] {
			switch {
			case f == "--recursive":
				recursive = true
			case f == "--force":
				force = true
			case strings.HasPrefix(f, "--"):
				// unrelated long flag; ignore
			case strings.HasPrefix(f, "-") && len(f) > 1:
				// combined/short flags such as -rf, -fr, -r, -f, -Rf
				for _, c := range f[1:] {
					if c == 'r' || c == 'R' {
						recursive = true
					}
					if c == 'f' {
						force = true
					}
				}
			default:
				if isRootTarget(f) {
					rootTarget = true
				}
			}
		}
		if recursive && force && rootTarget {
			return true
		}
	}
	return false
}

// CheckSafety returns a label and true if the command is dangerous.
func CheckSafety(cmd string) (string, bool) {
	for _, d := range dangerPatterns {
		if d.P.MatchString(cmd) {
			return d.L, true
		}
	}
	if isDangerousRm(cmd) {
		return "rm-rf-root", true
	}
	return "", false
}
