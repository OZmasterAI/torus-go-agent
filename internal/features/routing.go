// Package features provides heuristics and utilities used to determine
// how individual messages should be processed by the agent.
package features

import (
	"regexp"
	"strings"
)

// Thresholds for the simple-message classifier. A message that stays within
// both limits and contains none of the structural or semantic signals below
// is considered "simple" and can be handled by a lightweight model.
const (
	// maxSimpleBytes is the maximum byte length of a simple message.
	// Messages at or above this length are always routed to the capable model.
	maxSimpleBytes = 160

	// maxSimpleWords is the maximum word count of a simple message.
	// Messages at or above this word count are always routed to the capable model.
	maxSimpleWords = 28
)

// complexityKeywords are lowercase substrings whose presence signals that a
// message requires deeper reasoning, code generation, or multi-step planning.
// Matching is case-insensitive and substring-based (e.g. "implement" matches
// "reimplements").
var complexityKeywords = []string{
	"implement",
	"refactor",
	"debug",
	"analyze",
	"compare",
	"architecture",
	"design",
	"migrate",
	"optimize",
	"security",
}

// reCodeBlock matches triple-backtick code fences (```) anywhere in the text.
// Single or double backticks are not matched.
var reCodeBlock = regexp.MustCompile("```")

// reURL matches http:// or https:// URL prefixes (case-sensitive).
// Bare domains, IP addresses, and other protocols such as ftp:// are not matched.
var reURL = regexp.MustCompile(`https?://`)

// IsSimpleMessage reports whether text is simple enough to be handled by a
// lightweight model, avoiding the latency and cost of a full-capability model.
//
// A message is considered simple when ALL of the following hold:
//   - Byte length is below [maxSimpleBytes] (< 160 bytes)
//   - Word count is below [maxSimpleWords] (< 28 words)
//   - Text contains no triple-backtick code fences
//   - Text contains no http:// or https:// URLs
//   - Text contains none of the [complexityKeywords] (case-insensitive)
//
// Any single failing condition causes the message to be classified as complex
// and routed to the capable model.
func IsSimpleMessage(text string) bool {
	if len(text) >= maxSimpleBytes {
		return false
	}

	if len(strings.Fields(text)) >= maxSimpleWords {
		return false
	}

	if reCodeBlock.MatchString(text) {
		return false
	}

	if reURL.MatchString(text) {
		return false
	}

	lower := strings.ToLower(text)
	for _, kw := range complexityKeywords {
		if strings.Contains(lower, kw) {
			return false
		}
	}

	return true
}
