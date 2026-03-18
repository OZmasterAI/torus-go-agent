package core

import (
	"regexp"
	"strings"
)

// complexityKeywords are phrases that indicate a message needs a capable model.
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

// reCodeBlock matches triple-backtick code fences.
var reCodeBlock = regexp.MustCompile("```")

// reURL matches http:// or https:// URLs.
var reURL = regexp.MustCompile(`https?://`)

// IsSimpleMessage returns true when all of the following hold:
//   - text is under 160 characters
//   - text has fewer than 28 words
//   - text contains no code blocks (triple backticks)
//   - text contains no URLs
//   - text contains none of the complexity keywords
func IsSimpleMessage(text string) bool {
	if len(text) >= 160 {
		return false
	}

	words := strings.Fields(text)
	if len(words) >= 28 {
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

// RouteMessage returns the model ID that should handle this message.
//
// If SmartRouting is disabled (the default), the configured provider model is
// always returned. When SmartRouting is enabled and IsSimpleMessage is true,
// SmartRoutingModel is returned — falling back to the provider model when
// SmartRoutingModel is empty.
func RouteMessage(text string, config AgentConfig) string {
	if !config.SmartRouting {
		return config.Provider.Model
	}

	if IsSimpleMessage(text) {
		if config.SmartRoutingModel != "" {
			return config.SmartRoutingModel
		}
	}

	return config.Provider.Model
}
