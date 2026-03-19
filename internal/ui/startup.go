package ui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ProviderChoice holds a provider option for the startup menu.
type ProviderChoice struct {
	Name          string
	Provider      string // "openrouter", "nvidia", or "anthropic"
	Model         string
	NeedsKey      string // env var name
	ContextWindow int    // model's context window size
	MaxTokens     int    // max output tokens
}

// DefaultProviderChoices returns the standard options.
func DefaultProviderChoices() []ProviderChoice {
	return []ProviderChoice{
		{Name: "OpenRouter (hunter-alpha)", Provider: "openrouter", Model: "openrouter/hunter-alpha", NeedsKey: "OPENROUTER_API_KEY", ContextWindow: 128000, MaxTokens: 8192},
		{Name: "OpenRouter (nemotron-3-super)", Provider: "openrouter", Model: "nvidia/nemotron-3-super-120b-a12b:free", NeedsKey: "OPENROUTER_API_KEY", ContextWindow: 131072, MaxTokens: 8192},
		{Name: "OpenRouter (step-3.5-flash)", Provider: "openrouter", Model: "stepfun/step-3.5-flash:free", NeedsKey: "OPENROUTER_API_KEY", ContextWindow: 128000, MaxTokens: 8192},
		{Name: "NVIDIA NIM (GLM-4.7)", Provider: "nvidia", Model: "z-ai/glm4.7", NeedsKey: "NVIDIA_API_KEY", ContextWindow: 32768, MaxTokens: 8192},
		{Name: "NVIDIA NIM (Qwen3.5-122B)", Provider: "nvidia", Model: "qwen/qwen3.5-122b-a10b", NeedsKey: "NVIDIA_API_KEY", ContextWindow: 262144, MaxTokens: 16384},
		{Name: "NVIDIA NIM (llama-3.3-70b)", Provider: "nvidia", Model: "meta/llama-3.3-70b-instruct", NeedsKey: "NVIDIA_API_KEY", ContextWindow: 128000, MaxTokens: 8192},
		{Name: "Anthropic Claude (OAuth)", Provider: "anthropic", Model: "claude-sonnet-4-5-20250929", NeedsKey: "", ContextWindow: 200000, MaxTokens: 64000},
		{Name: "Anthropic Claude (API key)", Provider: "anthropic", Model: "claude-sonnet-4-5-20250929", NeedsKey: "ANTHROPIC_API_KEY", ContextWindow: 200000, MaxTokens: 64000},
		{Name: "Custom model", Provider: "", Model: "", NeedsKey: "", ContextWindow: 128000, MaxTokens: 8192},
	}
}

// RunStartup shows an interactive provider/model selection menu.
// Returns provider name and model ID. If skipStartup is true, returns empty strings.
func RunStartup(skipStartup bool) (provider, model string) {
	if skipStartup {
		return "", ""
	}

	choices := DefaultProviderChoices()

	fmt.Println()
	fmt.Println("  ◉ Torus Agent — Setup")
	fmt.Println("  ─────────────────────")
	fmt.Println()

	for i, c := range choices {
		available := ""
		if c.NeedsKey != "" && os.Getenv(c.NeedsKey) == "" {
			available = " (no key)"
		}
		if c.NeedsKey == "" && c.Provider == "anthropic" {
			available = " (will prompt OAuth)"
		}
		fmt.Printf("  %d) %s%s\n", i+1, c.Name, available)
	}
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("  Select [1-%d]: ", len(choices))
		if !scanner.Scan() {
			os.Exit(0)
		}
		input := strings.TrimSpace(scanner.Text())
		n, err := strconv.Atoi(input)
		if err != nil || n < 1 || n > len(choices) {
			fmt.Println("  Invalid choice.")
			continue
		}

		choice := choices[n-1]

		// Custom model
		if choice.Provider == "" {
			fmt.Print("  Provider (openrouter/anthropic/nvidia): ")
			scanner.Scan()
			provider = strings.TrimSpace(scanner.Text())
			fmt.Print("  Model ID: ")
			scanner.Scan()
			model = strings.TrimSpace(scanner.Text())
			return provider, model		}

		// Anthropic provider: let user pick the model
		if choice.Provider == "anthropic" {
			model = pickAnthropicModel(scanner)
			return choice.Provider, model		}

		return choice.Provider, choice.Model	}
}

func pickAnthropicModel(scanner *bufio.Scanner) string {
	models := []struct {
		Name string
		ID   string
	}{
		{"Claude Opus 4.6", "claude-opus-4-6"},
		{"Claude Sonnet 4.6", "claude-sonnet-4-6"},
		{"Claude Sonnet 4.5", "claude-sonnet-4-5-20250929"},
		{"Claude Haiku 4.5", "claude-haiku-4-5-20251001"},
		{"Custom model ID", ""},
	}

	fmt.Println()
	fmt.Println("  Select Anthropic model:")
	for i, m := range models {
		fmt.Printf("    %d) %s\n", i+1, m.Name)
	}
	fmt.Println()

	for {
		fmt.Printf("  Model [1-%d]: ", len(models))
		if !scanner.Scan() {
			return "claude-sonnet-4-5-20250929"
		}
		input := strings.TrimSpace(scanner.Text())
		n, err := strconv.Atoi(input)
		if err != nil || n < 1 || n > len(models) {
			fmt.Println("  Invalid choice.")
			continue
		}
		m := models[n-1]
		if m.ID == "" {
			fmt.Print("  Model ID: ")
			scanner.Scan()
			return strings.TrimSpace(scanner.Text())
		}
		return m.ID
	}
}
