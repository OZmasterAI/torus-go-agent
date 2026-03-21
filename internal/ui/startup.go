package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── OpenRouter model fetching ─────────────────────────────────────────────────

type openRouterModelResp struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Created       int64  `json:"created"`
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	TopProvider *struct {
		MaxCompletionTokens int `json:"max_completion_tokens"`
	} `json:"top_provider"`
	Architecture struct {
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
}

// fetchOpenRouterModels fetches all text models from the OpenRouter API and
// returns them as categories: FREE MODELS first, then one category per author.
// Returns nil on error (caller falls back to hardcoded defaults).
func fetchOpenRouterModels() []ModelCategory {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://openrouter.ai/api/v1/models")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var data openRouterModelResp
	if json.Unmarshal(body, &data) != nil {
		return nil
	}

	// Filter text-output models only
	var textModels []openRouterModel
	for _, m := range data.Data {
		for _, mod := range m.Architecture.OutputModalities {
			if mod == "text" {
				textModels = append(textModels, m)
				break
			}
		}
	}

	// Sort all by newest first
	sort.Slice(textModels, func(i, j int) bool {
		return textModels[i].Created > textModels[j].Created
	})

	now := time.Now().Unix()
	thirtyDays := int64(30 * 24 * 60 * 60)

	toChoice := func(m openRouterModel) ModelChoice {
		maxTokens := 0
		if m.TopProvider != nil {
			maxTokens = m.TopProvider.MaxCompletionTokens
		}
		tag := ""
		if now-m.Created < thirtyDays {
			tag = " [New]"
		}
		if m.Pricing.Prompt == "0" && m.Pricing.Completion == "0" {
			if tag == "" {
				tag = " [Free]"
			} else {
				tag += " [Free]"
			}
		}
		return ModelChoice{
			Name:          m.Name + tag,
			ID:            m.ID,
			ContextWindow: m.ContextLength,
			MaxTokens:     maxTokens,
		}
	}

	// Build FREE MODELS category
	var freeChoices []ModelChoice
	for _, m := range textModels {
		if m.Pricing.Prompt == "0" && m.Pricing.Completion == "0" {
			freeChoices = append(freeChoices, toChoice(m))
		}
	}
	freeChoices = append(freeChoices, ModelChoice{Name: "Custom model ID", ID: ""})

	// Build per-author categories
	authorOrder := []string{}
	authorModels := map[string][]ModelChoice{}
	for _, m := range textModels {
		author := m.ID
		if idx := strings.IndexByte(author, '/'); idx > 0 {
			author = author[:idx]
		}
		if _, seen := authorModels[author]; !seen {
			authorOrder = append(authorOrder, author)
		}
		authorModels[author] = append(authorModels[author], toChoice(m))
	}
	sort.Strings(authorOrder)

	// Assemble: FREE MODELS first, then alphabetical authors
	categories := []ModelCategory{
		{Name: "FREE MODELS", Models: freeChoices},
	}
	for _, author := range authorOrder {
		models := authorModels[author]
		models = append(models, ModelChoice{Name: "Custom model ID", ID: ""})
		categories = append(categories, ModelCategory{Name: author, Models: models})
	}

	return categories
}

// ── NVIDIA NIM model fetching ────────────────────────────────────────────────

type nvidiaNIMModelResp struct {
	Data []nvidiaNIMModel `json:"data"`
}

type nvidiaNIMModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// nvidiaNIMFreeModels lists model IDs confirmed free on build.nvidia.com.
var nvidiaNIMFreeModels = map[string]bool{
	"qwen/qwen3.5-122b-a10b":                       true,
	"z-ai/glm4.7":                                   true,
	"z-ai/glm5":                                     true,
	"stepfun-ai/step-3.5-flash":                     true,
	"minimaxai/minimax-m2.1":                        true,
	"minimaxai/minimax-m2.5":                        true,
	"deepseek-ai/deepseek-v3.2":                     true,
	"deepseek-ai/deepseek-v3.1":                     true,
	"deepseek-ai/deepseek-v3.1-terminus":            true,
	"mistralai/devstral-2-123b-instruct-2512":       true,
	"moonshotai/kimi-k2-thinking":                   true,
	"moonshotai/kimi-k2-instruct":                   true,
	"mistralai/mistral-large-3-675b-instruct-2512":  true,
	"mistralai/magistral-small-2506":                true,
	"mistralai/mamba-codestral-7b-v01":              true,
	"mistralai/mistral-nemo-minitron-8b-8k-instruct": true,
	"bytedance/seed-oss-36b-instruct":               true,
	"qwen/qwen3-coder-480b-a35b-instruct":           true,
	"openai/gpt-oss-20b":                            true,
	"openai/gpt-oss-120b":                           true,
	"google/gemma-3-27b-it":                         true,
	"google/gemma-2-2b-it":                          true,
	"google/gemma-3n-e4b-it":                        true,
	"google/shieldgemma-9b":                         true,
	"igenius/colosseum_355b_instruct_16k":            true,
	"tiiuae/falcon3-7b-instruct":                    true,
	"igenius/italia_10b_instruct_16k":                true,
	"nvidia/cosmos-nemotron-34b":                    true,
	"nvidia/cosmos-reason2-8b":                      true,
	"qwen/qwen2.5-coder-7b-instruct":               true,
	"qwen/qwen2-7b-instruct":                       true,
	"abacusai/dracarys-llama-3.1-70b-instruct":     true,
	"thudm/chatglm3-6b":                            true,
	"baichuan-inc/baichuan2-13b-chat":               true,
	"nvidia/nemotron-3-super-120b-a12b":             true,
	"nvidia/nemotron-3-nano-30b-a3b":                true,
	"nvidia/nvidia-nemotron-nano-9b-v2":             true,
	"nvidia/llama-3.3-nemotron-super-49b-v1":        true,
	"nvidia/llama-3.3-nemotron-super-49b-v1.5":      true,
	"nvidia/nemotron-content-safety-reasoning-4b":   true,
	"nvidia/llama-3.1-nemotron-safety-guard-8b-v3":  true,
	"nvidia/llama-3.1-nemotron-70b-reward":          true,
	"marin/marin-8b-instruct":                      true,
	"nv-mistralai/mistral-nemo-12b-instruct":        true,
}

// nvidiaNIMExcludeSubstrings lists substrings that identify non-chat models.
var nvidiaNIMExcludeSubstrings = []string{
	"embed", "bge", "nv-embed", "rerankqa", "reward", "neva", "nvclip",
	"streampetr", "deplot", "paligemma", "kosmos", "nemoretriever",
	"starcoder", "fuyu", "parse", "grounding-dino", "esm2", "diffdock",
	"molmim", "genomics", "riva", "voicechat", "studiovoice", "eyecontact",
	"parakeet", "canary", "vila", "cosmos-transfer", "genmol", "alphafold",
	"openfold", "msa-search", "sparsedrive", "bevformer", "usdsearch",
	"usdvalidate", "usdcode", "megatron-1b-nmt", "proteinmpnn",
	"ai-generated-image", "stable-diffusion", "flux", "trellis",
	"magpie-tts", "world-2", "arctic-embed",
}

// fetchNvidiaNIMModels fetches chat models from the NVIDIA NIM API and returns
// them as categories: FREE MODELS first, then one category per owner.
// Returns nil on error (caller falls back to hardcoded defaults).
func fetchNvidiaNIMModels() []ModelCategory {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://integrate.api.nvidia.com/v1/models")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var data nvidiaNIMModelResp
	if json.Unmarshal(body, &data) != nil {
		return nil
	}

	// Filter out non-chat models
	var chatModels []nvidiaNIMModel
	for _, m := range data.Data {
		lower := strings.ToLower(m.ID)
		excluded := false
		for _, sub := range nvidiaNIMExcludeSubstrings {
			if strings.Contains(lower, sub) {
				excluded = true
				break
			}
		}
		if !excluded {
			chatModels = append(chatModels, m)
		}
	}

	// Sort by newest first
	sort.Slice(chatModels, func(i, j int) bool {
		return chatModels[i].Created > chatModels[j].Created
	})

	toChoice := func(m nvidiaNIMModel) ModelChoice {
		tag := ""
		if nvidiaNIMFreeModels[m.ID] {
			tag = " [Free]"
		}
		return ModelChoice{
			Name:          m.ID + tag,
			ID:            m.ID,
			ContextWindow: 0,
			MaxTokens:     0,
		}
	}

	// Build FREE MODELS category
	var freeChoices []ModelChoice
	for _, m := range chatModels {
		if nvidiaNIMFreeModels[m.ID] {
			freeChoices = append(freeChoices, toChoice(m))
		}
	}
	freeChoices = append(freeChoices, ModelChoice{Name: "Custom model ID", ID: ""})

	// Build per-owner categories
	ownerOrder := []string{}
	ownerModels := map[string][]ModelChoice{}
	for _, m := range chatModels {
		owner := m.OwnedBy
		if owner == "" {
			// Fall back to prefix of ID
			if idx := strings.IndexByte(m.ID, '/'); idx > 0 {
				owner = m.ID[:idx]
			} else {
				owner = "other"
			}
		}
		if _, seen := ownerModels[owner]; !seen {
			ownerOrder = append(ownerOrder, owner)
		}
		ownerModels[owner] = append(ownerModels[owner], toChoice(m))
	}
	sort.Strings(ownerOrder)

	// Assemble: FREE MODELS first, then alphabetical owners
	categories := []ModelCategory{
		{Name: "FREE MODELS", Models: freeChoices},
	}
	for _, owner := range ownerOrder {
		models := ownerModels[owner]
		models = append(models, ModelChoice{Name: "Custom model ID", ID: ""})
		categories = append(categories, ModelCategory{Name: owner, Models: models})
	}

	return categories
}

// SetupResult holds the configuration choices from the setup screen.
type SetupResult struct {
	Provider string
	Model    string
	Config   *AgentConfigOverrides // nil means "use existing config.json"
}

// AgentConfigOverrides holds agent settings that can be set during setup.
type AgentConfigOverrides struct {
	MaxTokens              int
	ContextWindow          int
	Compaction             string
	CompactionTrigger      string
	CompactionThreshold    int
	CompactionMaxMessages  int
	CompactionKeepLastN    int
	ContinuousCompression  bool
	CompressionKeepLast    int
	CompressionMinMessages int
	ZoneBudgeting          bool
	ZoneArchivePercent     int
	SmartRouting           bool
	SteeringAggressive     bool
}

func defaultOverrides() *AgentConfigOverrides {
	return &AgentConfigOverrides{
		MaxTokens:              8192,
		ContextWindow:          128000,
		Compaction:             "llm",
		CompactionTrigger:      "both",
		CompactionThreshold:    75,
		CompactionMaxMessages:  50,
		CompactionKeepLastN:    20,
		ContinuousCompression:  true,
		CompressionKeepLast:    10,
		CompressionMinMessages: 0,
		ZoneBudgeting:          true,
		ZoneArchivePercent:     25,
		SmartRouting:           false,
		SteeringAggressive:     false,
	}
}

type configField struct {
	name string
	kind string // "bool", "int", "string"
}

var configFields = []configField{
	{"MaxTokens", "int"},
	{"ContextWindow", "int"},
	{"Compaction", "string"},
	{"CompactionTrigger", "string"},
	{"CompactionThreshold", "int"},
	{"CompactionMaxMessages", "int"},
	{"CompactionKeepLastN", "int"},
	{"ContinuousCompression", "bool"},
	{"CompressionKeepLast", "int"},
	{"CompressionMinMessages", "int"},
	{"ZoneBudgeting", "bool"},
	{"ZoneArchivePercent", "int"},
	{"SmartRouting", "bool"},
	{"SteeringAggressive", "bool"},
}

func (o *AgentConfigOverrides) getValue(idx int) string {
	switch idx {
	case 0:
		return fmt.Sprintf("%d", o.MaxTokens)
	case 1:
		return fmt.Sprintf("%d", o.ContextWindow)
	case 2:
		return o.Compaction
	case 3:
		return o.CompactionTrigger
	case 4:
		return fmt.Sprintf("%d", o.CompactionThreshold)
	case 5:
		return fmt.Sprintf("%d", o.CompactionMaxMessages)
	case 6:
		return fmt.Sprintf("%d", o.CompactionKeepLastN)
	case 7:
		if o.ContinuousCompression {
			return "true"
		}
		return "false"
	case 8:
		return fmt.Sprintf("%d", o.CompressionKeepLast)
	case 9:
		return fmt.Sprintf("%d", o.CompressionMinMessages)
	case 10:
		if o.ZoneBudgeting {
			return "true"
		}
		return "false"
	case 11:
		return fmt.Sprintf("%d", o.ZoneArchivePercent)
	case 12:
		if o.SmartRouting {
			return "true"
		}
		return "false"
	case 13:
		if o.SteeringAggressive {
			return "true"
		}
		return "false"
	}
	return ""
}

func (o *AgentConfigOverrides) setValue(idx int, val string) {
	n, _ := strconv.Atoi(val)
	switch idx {
	case 0:
		o.MaxTokens = n
	case 1:
		o.ContextWindow = n
	case 2:
		o.Compaction = val
	case 3:
		o.CompactionTrigger = val
	case 4:
		o.CompactionThreshold = n
	case 5:
		o.CompactionMaxMessages = n
	case 6:
		o.CompactionKeepLastN = n
	case 7:
		o.ContinuousCompression = val == "true"
	case 8:
		o.CompressionKeepLast = n
	case 9:
		o.CompressionMinMessages = n
	case 10:
		o.ZoneBudgeting = val == "true"
	case 11:
		o.ZoneArchivePercent = n
	case 12:
		o.SmartRouting = val == "true"
	case 13:
		o.SteeringAggressive = val == "true"
	}
}

func (o *AgentConfigOverrides) toggleBool(idx int) {
	switch idx {
	case 7:
		o.ContinuousCompression = !o.ContinuousCompression
	case 10:
		o.ZoneBudgeting = !o.ZoneBudgeting
	case 12:
		o.SmartRouting = !o.SmartRouting
	case 13:
		o.SteeringAggressive = !o.SteeringAggressive
	}
}

// AuthMethod represents an authentication option for a provider.
type AuthMethod struct {
	Name     string // display name, e.g. "OAuth", "API key"
	NeedsKey string // env var name ("" for OAuth)
}

// ModelCategory groups models under a named category (e.g. "FREE MODELS").
// Providers with categories get an extra selection step: provider → category → model.
type ModelCategory struct {
	Name   string
	Models []ModelChoice
}

// ProviderGroup groups models and auth methods under a single provider.
type ProviderGroup struct {
	Name        string           // display name, e.g. "Anthropic Claude"
	ProviderKey string           // "anthropic", "openrouter", etc. ("" = custom)
	AuthMethods []AuthMethod     // if len > 1, user picks auth before model
	Models      []ModelChoice    // direct models (used when Categories is empty)
	Categories  []ModelCategory  // if set, user picks category before model
}

// ModelChoice is a single model within a provider group.
type ModelChoice struct {
	Name          string
	ID            string // model ID sent to API
	ContextWindow int
	MaxTokens     int
}

// DefaultProviderGroups returns the grouped provider options.
func DefaultProviderGroups() []ProviderGroup {
	return []ProviderGroup{
		{
			Name: "OpenRouter", ProviderKey: "openrouter",
			AuthMethods: []AuthMethod{{Name: "API key", NeedsKey: "OPENROUTER_API_KEY"}},
			Categories: []ModelCategory{
				{Name: "FREE MODELS", Models: []ModelChoice{
					{Name: "nemotron-3-super (free)", ID: "nvidia/nemotron-3-super-120b-a12b:free", ContextWindow: 131072, MaxTokens: 8192},
					{Name: "step-3.5-flash (free)", ID: "stepfun/step-3.5-flash:free", ContextWindow: 128000, MaxTokens: 8192},
					{Name: "Custom model ID", ID: ""},
				}},
			},
		},
		{
			Name: "NVIDIA NIM", ProviderKey: "nvidia",
			AuthMethods: []AuthMethod{{Name: "API key", NeedsKey: "NVIDIA_API_KEY"}},
			Categories: []ModelCategory{
				{Name: "FREE MODELS", Models: []ModelChoice{
					{Name: "GLM-4.7", ID: "z-ai/glm4.7"},
					{Name: "Qwen3.5-122B", ID: "qwen/qwen3.5-122b-a10b"},
					{Name: "llama-3.3-70b-instruct", ID: "meta/llama-3.3-70b-instruct"},
					{Name: "Custom model ID", ID: ""},
				}},
			},
		},
		{
			Name: "Anthropic Claude", ProviderKey: "anthropic",
			AuthMethods: []AuthMethod{
				{Name: "OAuth (no key needed)", NeedsKey: ""},
				{Name: "API key", NeedsKey: "ANTHROPIC_API_KEY"},
			},
			Models: []ModelChoice{
				{Name: "Claude Opus 4.6", ID: "claude-opus-4-6", ContextWindow: 1000000, MaxTokens: 128000},
				{Name: "Claude Sonnet 4.6", ID: "claude-sonnet-4-6", ContextWindow: 1000000, MaxTokens: 64000},
				{Name: "Claude Haiku 4.5", ID: "claude-haiku-4-5-20251001", ContextWindow: 200000, MaxTokens: 64000},
				{Name: "Claude Sonnet 4.5", ID: "claude-sonnet-4-5-20250929", ContextWindow: 1000000, MaxTokens: 64000},
				{Name: "Claude Opus 4.5", ID: "claude-opus-4-5-20251101", ContextWindow: 200000, MaxTokens: 64000},
				{Name: "Claude Opus 4.1", ID: "claude-opus-4-1-20250805", ContextWindow: 200000, MaxTokens: 32000},
				{Name: "Claude Sonnet 4", ID: "claude-sonnet-4-20250514", ContextWindow: 1000000, MaxTokens: 64000},
				{Name: "Claude Opus 4", ID: "claude-opus-4-20250514", ContextWindow: 200000, MaxTokens: 32000},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name: "OpenAI", ProviderKey: "openai",
			AuthMethods: []AuthMethod{{Name: "API key", NeedsKey: "OPENAI_API_KEY"}},
			Models: []ModelChoice{
				{Name: "GPT-5.4", ID: "gpt-5.4", ContextWindow: 1000000, MaxTokens: 128000},
				{Name: "GPT-5.4 Mini", ID: "gpt-5.4-mini", ContextWindow: 400000, MaxTokens: 128000},
				{Name: "GPT-5.4 Nano", ID: "gpt-5.4-nano", ContextWindow: 400000, MaxTokens: 128000},
				{Name: "GPT-4.1", ID: "gpt-4.1", ContextWindow: 1047576, MaxTokens: 32768},
				{Name: "GPT-4.1 Mini", ID: "gpt-4.1-mini", ContextWindow: 1047576, MaxTokens: 32768},
				{Name: "GPT-4.1 Nano", ID: "gpt-4.1-nano", ContextWindow: 1047576, MaxTokens: 32768},
				{Name: "GPT-4o", ID: "gpt-4o", ContextWindow: 128000, MaxTokens: 16384},
				{Name: "GPT-4o Mini", ID: "gpt-4o-mini", ContextWindow: 128000, MaxTokens: 16384},
				{Name: "o4-mini", ID: "o4-mini", ContextWindow: 200000, MaxTokens: 100000},
				{Name: "o3", ID: "o3", ContextWindow: 200000, MaxTokens: 100000},
				{Name: "o3 Mini", ID: "o3-mini", ContextWindow: 200000, MaxTokens: 100000},
				{Name: "o3 Pro", ID: "o3-pro", ContextWindow: 200000, MaxTokens: 100000},
				{Name: "o1", ID: "o1", ContextWindow: 200000, MaxTokens: 100000},
				{Name: "o1 Mini", ID: "o1-mini", ContextWindow: 128000, MaxTokens: 65536},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name: "Grok (xAI)", ProviderKey: "grok",
			AuthMethods: []AuthMethod{{Name: "API key", NeedsKey: "XAI_API_KEY"}},
			Models: []ModelChoice{
				{Name: "Grok 4.20 (reasoning)", ID: "grok-4.20-0309-reasoning", ContextWindow: 2000000, MaxTokens: 131072},
				{Name: "Grok 4.20 (non-reasoning)", ID: "grok-4.20-0309-non-reasoning", ContextWindow: 2000000, MaxTokens: 131072},
				{Name: "Grok 4.1 Fast (reasoning)", ID: "grok-4-1-fast-reasoning", ContextWindow: 2000000, MaxTokens: 131072},
				{Name: "Grok 4.1 Fast (non-reasoning)", ID: "grok-4-1-fast-non-reasoning", ContextWindow: 2000000, MaxTokens: 131072},
				{Name: "grok-3-mini", ID: "grok-3-mini", ContextWindow: 131072, MaxTokens: 8192},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name: "Google Gemini", ProviderKey: "gemini",
			AuthMethods: []AuthMethod{{Name: "API key", NeedsKey: "GEMINI_API_KEY"}},
			Models: []ModelChoice{
				{Name: "Gemini 3.1 Pro (preview)", ID: "gemini-3.1-pro-preview", ContextWindow: 1048576, MaxTokens: 65536},
				{Name: "Gemini 3.1 Flash Lite (preview) [Free]", ID: "gemini-3.1-flash-lite-preview", ContextWindow: 1048576, MaxTokens: 65536},
				{Name: "Gemini 3 Flash (preview) [Free]", ID: "gemini-3-flash-preview", ContextWindow: 1048576, MaxTokens: 65536},
				{Name: "Gemini 2.5 Pro", ID: "gemini-2.5-pro", ContextWindow: 1048576, MaxTokens: 65535},
				{Name: "Gemini 2.5 Flash [Free]", ID: "gemini-2.5-flash", ContextWindow: 1048576, MaxTokens: 65535},
				{Name: "Gemini 2.5 Flash Lite [Free]", ID: "gemini-2.5-flash-lite", ContextWindow: 1048576, MaxTokens: 65535},
				{Name: "Gemini 2.0 Flash [Free]", ID: "gemini-2.0-flash", ContextWindow: 1000000, MaxTokens: 65536},
				{Name: "Gemini 2.0 Flash Lite [Free]", ID: "gemini-2.0-flash-lite", ContextWindow: 1000000, MaxTokens: 65536},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name: "Azure OpenAI", ProviderKey: "azure",
			AuthMethods: []AuthMethod{{Name: "API key", NeedsKey: "AZURE_OPENAI_API_KEY"}},
			Models: []ModelChoice{
				{Name: "GPT-5.4", ID: "gpt-5.4", ContextWindow: 1000000, MaxTokens: 128000},
				{Name: "GPT-5.4 Mini", ID: "gpt-5.4-mini", ContextWindow: 400000, MaxTokens: 128000},
				{Name: "GPT-4.1", ID: "gpt-4.1", ContextWindow: 1047576, MaxTokens: 32768},
				{Name: "GPT-4.1 Mini", ID: "gpt-4.1-mini", ContextWindow: 1047576, MaxTokens: 32768},
				{Name: "GPT-4o", ID: "gpt-4o", ContextWindow: 128000, MaxTokens: 16384},
				{Name: "o4-mini", ID: "o4-mini", ContextWindow: 200000, MaxTokens: 100000},
				{Name: "o3", ID: "o3", ContextWindow: 200000, MaxTokens: 100000},
				{Name: "o3 Mini", ID: "o3-mini", ContextWindow: 200000, MaxTokens: 100000},
				{Name: "Custom deployment", ID: ""},
			},
		},
		{
			Name: "Vertex AI", ProviderKey: "vertex",
			AuthMethods: []AuthMethod{{Name: "Access token", NeedsKey: "VERTEX_ACCESS_TOKEN"}},
			Models: []ModelChoice{
				{Name: "Gemini 3.1 Pro (preview)", ID: "gemini-3.1-pro-preview", ContextWindow: 1048576, MaxTokens: 65536},
				{Name: "Gemini 3 Flash (preview)", ID: "gemini-3-flash-preview", ContextWindow: 1048576, MaxTokens: 65536},
				{Name: "Gemini 2.5 Pro", ID: "gemini-2.5-pro", ContextWindow: 1048576, MaxTokens: 65535},
				{Name: "Gemini 2.5 Flash", ID: "gemini-2.5-flash", ContextWindow: 1048576, MaxTokens: 65535},
				{Name: "Gemini 2.5 Flash Lite", ID: "gemini-2.5-flash-lite", ContextWindow: 1048576, MaxTokens: 65535},
				{Name: "Gemini 2.0 Flash", ID: "gemini-2.0-flash", ContextWindow: 1000000, MaxTokens: 65536},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name: "Custom provider", ProviderKey: "",
		},
	}
}

// ── Color palette (amber/orange) ──────────────────────────────────────────────

var (
	colorBrightAmber = lipgloss.Color("166")
	colorOrange      = lipgloss.Color("130")
	colorMutedGold   = lipgloss.Color("130")
)

// Styles derived from the palette.
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(colorBrightAmber).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorMutedGold).
			Italic(true)

	menuPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorOrange).
			Padding(1, 2)

	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(colorBrightAmber).
				Bold(true).
				Padding(0, 1)

	menuHeaderStyle = lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true).
			Underline(true).
			MarginBottom(1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242")).
			Italic(true)

	// Torus character luminance styles (neon orange gradient).
	torusDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#993300")) // dim ember
	torusMidStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#cc4400")) // warm orange
	torusBrightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4d01")) // neon orange
	torusMaxStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6600")) // hot orange

	textInputStyle = lipgloss.NewStyle().
			Foreground(colorBrightAmber).
			Bold(true)

	promptLabelStyle = lipgloss.NewStyle().
				Foreground(colorMutedGold)

	scrollIndicatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242")).
				Italic(true)
)

// Maximum number of menu items visible before scrolling kicks in.
const visibleItems = 10

// ── ASCII art title ───────────────────────────────────────────────────────────

const asciiTitle = ` ████████╗  ██████╗  ██████╗  ██╗   ██╗ ███████╗
 ╚══██╔══╝ ██╔═══██╗ ██╔══██╗ ██║   ██║ ██╔════╝
    ██║    ██║   ██║ ██████╔╝ ██║   ██║ ███████╗
    ██║    ██║   ██║ ██╔══██╗ ██║   ██║ ╚════██║
    ██║    ╚██████╔╝ ██║  ██║ ╚██████╔╝ ███████║
    ╚═╝     ╚═════╝  ╚═╝  ╚═╝  ╚═════╝  ╚══════╝
         ╔═╗ ╔═╗  ╔═╗ ╔═╗ ╔═╗ ╔╗╔ ╔╦╗
         ║ ╦ ║ ║  ╠═╣ ║ ╦ ║╣  ║║║  ║
         ╚═╝ ╚═╝  ╚ ╚ ╚═╝ ╚═╝ ╝╚╝  ╩`

// Neon orange gradient colors for title animation (wave effect).
var titleGradient = []lipgloss.Color{
	lipgloss.Color("#662200"), // dark ember
	lipgloss.Color("#993300"), // dim orange
	lipgloss.Color("#cc4400"), // warm orange
	lipgloss.Color("#ff4d01"), // neon orange
	lipgloss.Color("#ff6600"), // hot orange
	lipgloss.Color("#ff8800"), // bright orange
	lipgloss.Color("#ffaa00"), // gold
	lipgloss.Color("#ff8800"), // bright orange
	lipgloss.Color("#ff6600"), // hot orange
	lipgloss.Color("#ff4d01"), // neon orange
	lipgloss.Color("#cc4400"), // warm orange
	lipgloss.Color("#993300"), // dim orange
}

// renderAnimatedTitle colors each column of the ASCII title with a wave
// of neon orange that shifts over time based on the torus rotation angle.
func renderAnimatedTitle(title string, phase float64) string {
	lines := strings.Split(title, "\n")
	gradLen := len(titleGradient)
	var sb strings.Builder
	for _, line := range lines {
		runes := []rune(line)
		for col, ch := range runes {
			if ch == ' ' {
				sb.WriteByte(' ')
				continue
			}
			// Wave: shift color index by column + phase
			idx := (col + int(phase*3)) % gradLen
			if idx < 0 {
				idx += gradLen
			}
			style := lipgloss.NewStyle().Foreground(titleGradient[idx]).Bold(true)
			sb.WriteString(style.Render(string(ch)))
		}
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ── Tick command for startup animation ────────────────────────────────────────
// tickMsg is declared in tui.go as `type tickMsg time.Time` (same package).

func startupTickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ── Setup model ───────────────────────────────────────────────────────────────

type setupModel struct {
	width, height int
	ready         bool

	// Torus animation
	torusA, torusB float64
	torusFrame     string

	// Menu phases:
	// 0=main, 1=pick provider, 2=pick auth, 3=pick category (if any), 4=pick model, 5=config mode, 6=edit settings
	phase        int
	cursor       int
	scrollOffset int // first visible item index for scrollable menus

	// Provider groups (phase 1)
	groups []ProviderGroup

	// Current selection state
	selectedGroup    *ProviderGroup  // set after phase 1
	selectedAuth     *AuthMethod     // set after phase 2 (or auto-set if single auth)
	selectedCategory *ModelCategory  // set after phase 3 (or nil if no categories)

	// Text input for custom provider/model
	textInput string
	inputMode bool // true when typing custom provider/model
	inputStep int  // 0=provider name, 1=model id

	// Config customization (phase 5 = choose config mode, phase 6 = edit settings)
	configOverrides *AgentConfigOverrides
	configCursor    int    // cursor for config editing in phase 6
	editingConfig   bool   // true when editing a numeric/string value
	editBuffer      string // text buffer for numeric/string input

	// Result
	provider string
	model    string
	done     bool
}

func newSetupModel() setupModel {
	groups := DefaultProviderGroups()

	// Fetch live OpenRouter models; replace all categories on success
	if cats := fetchOpenRouterModels(); len(cats) > 0 {
		for i := range groups {
			if groups[i].ProviderKey == "openrouter" {
				groups[i].Categories = cats
				break
			}
		}
	}

	// Fetch live NVIDIA NIM models; replace all categories on success
	if cats := fetchNvidiaNIMModels(); len(cats) > 0 {
		for i := range groups {
			if groups[i].ProviderKey == "nvidia" {
				groups[i].Categories = cats
				break
			}
		}
	}

	m := setupModel{groups: groups}
	m.torusFrame = renderTorus(m.torusA, m.torusB)
	return m
}

func (m setupModel) Init() tea.Cmd {
	return startupTickCmd()
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil

	case tickMsg:
		m.torusA += 0.04
		m.torusB += 0.02
		m.torusFrame = renderTorus(m.torusA, m.torusB)
		return m, startupTickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m setupModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ── Config editing mode (phase 5 inline edit) ────────────────
	if m.editingConfig {
		return m.handleConfigEdit(msg)
	}

	// ── Text input mode ──────────────────────────────────────────
	if m.inputMode {
		return m.handleTextInput(msg)
	}

	// ── Normal menu navigation ───────────────────────────────────
	switch msg.String() {

	case "ctrl+c", "q":
		m.provider = ""
		m.model = ""
		m.done = true
		return m, tea.Quit

	case "up", "k":
		total := m.menuLen()
		if m.cursor > 0 {
			m.cursor--
		} else {
			// Wrap to bottom
			m.cursor = total - 1
		}
		m.scrollOffset = clampScrollOffset(m.cursor, m.scrollOffset, total)

	case "down", "j":
		total := m.menuLen()
		if m.cursor < total-1 {
			m.cursor++
		} else {
			// Wrap to top
			m.cursor = 0
		}
		m.scrollOffset = clampScrollOffset(m.cursor, m.scrollOffset, total)

	case "enter":
		return m.selectItem()

	case "esc":
		switch m.phase {
		case 6: // settings → config mode
			m.phase = 5
			m.cursor = 0
			m.scrollOffset = 0
		case 5: // config mode → model select
			m.phase = 4
			m.cursor = 0
			m.scrollOffset = 0
		case 4: // model → category (if any) or auth/provider
			if m.selectedCategory != nil {
				m.phase = 3
			} else if m.selectedGroup != nil && len(m.selectedGroup.AuthMethods) > 1 {
				m.phase = 2
			} else {
				m.phase = 1
			}
			m.cursor = 0
			m.scrollOffset = 0
		case 3: // category → auth (if multi-auth) or provider
			m.selectedCategory = nil
			if m.selectedGroup != nil && len(m.selectedGroup.AuthMethods) > 1 {
				m.phase = 2
			} else {
				m.phase = 1
			}
			m.cursor = 0
			m.scrollOffset = 0
		case 2: // auth → provider
			m.phase = 1
			m.cursor = 0
			m.scrollOffset = 0
			m.selectedAuth = nil
		case 1: // provider → main
			m.phase = 0
			m.cursor = 0
			m.scrollOffset = 0
			m.selectedGroup = nil
		}
	}

	return m, nil
}

func (m setupModel) handleConfigEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.editBuffer)
		if val != "" {
			m.configOverrides.setValue(m.cursor, val)
		}
		m.editingConfig = false
		m.editBuffer = ""
	case "esc":
		m.editingConfig = false
		m.editBuffer = ""
	case "backspace":
		if len(m.editBuffer) > 0 {
			m.editBuffer = m.editBuffer[:len(m.editBuffer)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.editBuffer += msg.String()
		}
	}
	return m, nil
}

func (m setupModel) handleTextInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {

	case "ctrl+c":
		m.provider = ""
		m.model = ""
		m.done = true
		return m, tea.Quit

	case "esc":
		m.inputMode = false
		m.textInput = ""
		m.inputStep = 0
		return m, nil

	case "enter":
		val := strings.TrimSpace(m.textInput)
		if val == "" {
			return m, nil
		}
		// Custom provider flow (phase 1): step 0 = provider name, step 1 = model ID
		if m.phase == 1 {
			if m.inputStep == 0 {
				m.provider = val
				m.textInput = ""
				m.inputStep = 1
				return m, nil
			}
			// step 1: got model ID → go to config phase
			m.model = val
			m.inputMode = false
			m.textInput = ""
			m.inputStep = 0
			m.phase = 5
			m.cursor = 0
			m.scrollOffset = 0
			m.configOverrides = defaultOverrides()
			return m, nil
		}
		// Custom model ID within a provider (phase 4)
		if m.phase == 4 {
			m.provider = m.selectedGroup.ProviderKey
			m.model = val
			m.inputMode = false
			m.textInput = ""
			m.phase = 5
			m.cursor = 0
			m.scrollOffset = 0
			m.configOverrides = defaultOverrides()
			return m, nil
		}
		return m, nil

	case "backspace":
		if len(m.textInput) > 0 {
			m.textInput = m.textInput[:len(m.textInput)-1]
		}

	default:
		// Only accept printable single characters
		if len(msg.String()) == 1 {
			m.textInput += msg.String()
		}
	}

	return m, nil
}

// currentModels returns the model list for the current selection context.
// If a category is selected, returns that category's models; otherwise the group's direct models.
func (m setupModel) currentModels() []ModelChoice {
	if m.selectedCategory != nil {
		return m.selectedCategory.Models
	}
	if m.selectedGroup != nil {
		return m.selectedGroup.Models
	}
	return nil
}

func (m setupModel) menuLen() int {
	switch m.phase {
	case 0:
		return 2
	case 1:
		return len(m.groups)
	case 2:
		if m.selectedGroup != nil {
			return len(m.selectedGroup.AuthMethods)
		}
		return 0
	case 3: // categories
		if m.selectedGroup != nil {
			return len(m.selectedGroup.Categories)
		}
		return 0
	case 4: // models
		return len(m.currentModels())
	case 5:
		return 3
	case 6:
		return len(configFields) + 1
	}
	return 0
}

// clampScrollOffset returns an adjusted scrollOffset that keeps cursor within
// the visible window.  For short lists (total <= visibleItems) it returns 0.
func clampScrollOffset(cursor, scrollOffset, total int) int {
	if total <= visibleItems {
		return 0
	}
	// Cursor above the viewport — scroll up.
	if cursor < scrollOffset {
		return cursor
	}
	// Cursor below the viewport — scroll down.
	if cursor >= scrollOffset+visibleItems {
		return cursor - visibleItems + 1
	}
	return scrollOffset
}

// renderScrollableItems writes a windowed slice of items into the builder,
// adding scroll indicators when there are hidden items above or below.
// labelFn is called for each index in [0, total) and must return the display
// string for that item (without the "> " / "  " prefix).
func (m setupModel) renderScrollableItems(b *strings.Builder, total int, labelFn func(i int) string) {
	if total == 0 {
		return
	}

	start := m.scrollOffset
	end := start + visibleItems
	if end > total || total <= visibleItems {
		end = total
		start = end - visibleItems
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		b.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ... %d more above", start)))
		b.WriteByte('\n')
	}

	for i := start; i < end; i++ {
		label := labelFn(i)
		if i == m.cursor {
			b.WriteString(menuSelectedStyle.Render("> " + label))
		} else {
			b.WriteString(menuItemStyle.Render("  " + label))
		}
		b.WriteByte('\n')
	}

	if end < total {
		b.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ... %d more below", total-end)))
		b.WriteByte('\n')
	}
}

// renderScrollableSettings renders phase-6 config fields with scroll support.
// It needs its own method because config items have custom rendering (bool toggles,
// inline editing, and the trailing "Done" button).
func (m setupModel) renderScrollableSettings(b *strings.Builder, total int) {
	if total == 0 {
		return
	}

	start := m.scrollOffset
	end := start + visibleItems
	if end > total || total <= visibleItems {
		end = total
		start = end - visibleItems
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		b.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ... %d more above", start)))
		b.WriteByte('\n')
	}

	doneIdx := len(configFields)
	for i := start; i < end; i++ {
		if i == doneIdx {
			// "Done" button
			if m.cursor == doneIdx {
				b.WriteString(menuSelectedStyle.Render("> Done"))
			} else {
				b.WriteString(menuItemStyle.Render("  Done"))
			}
			b.WriteByte('\n')
			continue
		}
		f := configFields[i]
		val := m.configOverrides.getValue(i)
		var line string
		if f.kind == "bool" {
			indicator := "○"
			if val == "true" {
				indicator = "●"
			}
			line = fmt.Sprintf("%s %s", indicator, f.name)
		} else {
			line = fmt.Sprintf("%s: %s", f.name, val)
		}
		if i == m.cursor {
			if m.editingConfig {
				line = fmt.Sprintf("%s: %s_", f.name, m.editBuffer)
				b.WriteString(textInputStyle.Render("> " + line))
			} else {
				b.WriteString(menuSelectedStyle.Render("> " + line))
			}
		} else {
			b.WriteString(menuItemStyle.Render("  " + line))
		}
		b.WriteByte('\n')
	}

	if end < total {
		b.WriteString(scrollIndicatorStyle.Render(fmt.Sprintf("  ... %d more below", total-end)))
		b.WriteByte('\n')
	}
}

// nextModelPhase returns the phase to go to after auth selection.
// If the provider has categories, go to category select (3); otherwise model select (4).
func (m setupModel) nextModelPhase() int {
	if m.selectedGroup != nil && len(m.selectedGroup.Categories) > 0 {
		return 3
	}
	return 4
}

func (m setupModel) selectItem() (tea.Model, tea.Cmd) {
	switch m.phase {

	case 0: // Main menu
		switch m.cursor {
		case 0: // Use existing config
			m.provider = ""
			m.model = ""
			m.done = true
			return m, tea.Quit
		case 1: // Choose provider & model
			m.phase = 1
			m.cursor = 0
			m.scrollOffset = 0
		}

	case 1: // Pick provider group
		group := m.groups[m.cursor]
		if group.ProviderKey == "" {
			m.inputMode = true
			m.inputStep = 0
			m.textInput = ""
			return m, nil
		}
		m.selectedGroup = &group
		if len(group.AuthMethods) > 1 {
			m.phase = 2
			m.cursor = 0
			m.scrollOffset = 0
		} else {
			m.selectedAuth = &group.AuthMethods[0]
			m.phase = m.nextModelPhase()
			m.cursor = 0
			m.scrollOffset = 0
		}
		return m, nil

	case 2: // Pick auth method
		auth := m.selectedGroup.AuthMethods[m.cursor]
		m.selectedAuth = &auth
		m.phase = m.nextModelPhase()
		m.cursor = 0
		m.scrollOffset = 0
		return m, nil

	case 3: // Pick category
		cat := m.selectedGroup.Categories[m.cursor]
		m.selectedCategory = &cat
		m.phase = 4
		m.cursor = 0
		m.scrollOffset = 0
		return m, nil

	case 4: // Pick model
		models := m.currentModels()
		mc := models[m.cursor]
		if mc.ID == "" {
			m.inputMode = true
			m.textInput = ""
			return m, nil
		}
		m.provider = m.selectedGroup.ProviderKey
		m.model = mc.ID
		m.phase = 5
		m.cursor = 0
		m.scrollOffset = 0
		m.configOverrides = defaultOverrides()
		if mc.ContextWindow > 0 {
			m.configOverrides.ContextWindow = mc.ContextWindow
		}
		if mc.MaxTokens > 0 {
			m.configOverrides.MaxTokens = mc.MaxTokens
		}
		return m, nil

	case 5: // Config mode
		switch m.cursor {
		case 0:
			m.configOverrides = defaultOverrides()
			m.done = true
			return m, tea.Quit
		case 1:
			m.configOverrides = nil
			m.done = true
			return m, tea.Quit
		case 2:
			if m.configOverrides == nil {
				m.configOverrides = defaultOverrides()
			}
			m.phase = 6
			m.cursor = 0
			m.scrollOffset = 0
			m.configCursor = 0
		}

	case 6: // Edit config
		if m.cursor >= len(configFields) {
			m.done = true
			return m, tea.Quit
		}
		f := configFields[m.cursor]
		if f.kind == "bool" {
			m.configOverrides.toggleBool(m.cursor)
		} else {
			m.editingConfig = true
			m.editBuffer = m.configOverrides.getValue(m.cursor)
		}
	}

	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m setupModel) View() string {
	if !m.ready {
		return ""
	}

	var b strings.Builder

	// ── Title block (animated neon gradient) ─────────────────────
	titleRendered := renderAnimatedTitle(asciiTitle, m.torusA)
	titleBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, titleRendered)
	b.WriteString(titleBlock)
	b.WriteString("\n\n")

	// ── Menu panel ────────────────────────────────────────────────
	menuContent := m.renderMenu()
	menuPanel := menuPanelStyle.Render(menuContent)

	// ── Layout: torus left, menu right (or menu only if narrow) ──
	narrow := m.width < 80
	if narrow {
		// Center the menu panel only
		centered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, menuPanel)
		b.WriteString(centered)
	} else {
		// Color the torus frame
		coloredTorus := colorTorus(m.torusFrame)

		torusPanel := lipgloss.NewStyle().
			Width(38).
			Render(coloredTorus)

		joined := lipgloss.JoinHorizontal(lipgloss.Top, torusPanel, "  ", menuPanel)
		centered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, joined)
		b.WriteString(centered)
	}

	b.WriteString("\n\n")

	// ── Footer hints ──────────────────────────────────────────────
	var hint string
	if m.inputMode {
		hint = "enter: confirm  |  esc: cancel  |  ctrl+c: quit"
	} else {
		phaseHints := []string{
			"j/k or arrows: navigate  |  enter: select  |  q: quit",                // 0: main
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",  // 1: provider
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",  // 2: auth
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",  // 3: category
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",  // 4: model
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",  // 5: config
			"j/k: navigate  |  enter: toggle/edit  |  esc: back  |  q: quit",       // 6: settings
		}
		if m.phase < len(phaseHints) {
			hint = phaseHints[m.phase]
		}
	}
	footer := footerStyle.Render(hint)
	footerBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer)
	b.WriteString(footerBlock)

	return b.String()
}

func (m setupModel) renderMenu() string {
	var b strings.Builder

	switch m.phase {
	case 0: // Main menu
		b.WriteString(menuHeaderStyle.Render("Setup"))
		b.WriteByte('\n')
		items := []string{
			"Use existing config",
			"Choose provider & model",
		}
		m.renderScrollableItems(&b, len(items), func(i int) string {
			return items[i]
		})

	case 1: // Pick provider
		b.WriteString(menuHeaderStyle.Render("Select Provider"))
		b.WriteByte('\n')
		m.renderScrollableItems(&b, len(m.groups), func(i int) string {
			return m.groups[i].Name
		})

	case 2: // Pick auth method
		header := "Authentication"
		if m.selectedGroup != nil {
			header = m.selectedGroup.Name + " — Authentication"
		}
		b.WriteString(menuHeaderStyle.Render(header))
		b.WriteByte('\n')
		if m.selectedGroup != nil {
			auths := m.selectedGroup.AuthMethods
			m.renderScrollableItems(&b, len(auths), func(i int) string {
				return auths[i].Name
			})
		}

	case 3: // Pick category
		header := "Select Category"
		if m.selectedGroup != nil {
			header = m.selectedGroup.Name + " — Select Category"
		}
		b.WriteString(menuHeaderStyle.Render(header))
		b.WriteByte('\n')
		if m.selectedGroup != nil {
			cats := m.selectedGroup.Categories
			m.renderScrollableItems(&b, len(cats), func(i int) string {
				return fmt.Sprintf("%s (%d)", cats[i].Name, len(cats[i].Models))
			})
		}

	case 4: // Pick model
		header := "Select Model"
		if m.selectedGroup != nil {
			if m.selectedCategory != nil {
				header = m.selectedGroup.Name + " — " + m.selectedCategory.Name
			} else {
				header = m.selectedGroup.Name + " — Select Model"
			}
		}
		b.WriteString(menuHeaderStyle.Render(header))
		b.WriteByte('\n')
		models := m.currentModels()
		m.renderScrollableItems(&b, len(models), func(i int) string {
			return models[i].Name
		})

	case 5: // Config mode
		b.WriteString(menuHeaderStyle.Render("Configuration"))
		b.WriteByte('\n')
		items := []string{
			"Use defaults",
			"Use existing config",
			"Customize settings",
		}
		m.renderScrollableItems(&b, len(items), func(i int) string {
			return items[i]
		})

	case 6: // Edit settings
		b.WriteString(menuHeaderStyle.Render("Settings"))
		b.WriteByte('\n')
		total := len(configFields) + 1 // +1 for "Done →"
		m.renderScrollableSettings(&b, total)
	}

	// ── Text input overlay ────────────────────────────────────────
	if m.inputMode {
		b.WriteByte('\n')
		var label string
		switch {
		case m.phase == 1 && m.inputStep == 0:
			label = "Provider name: "
		case m.phase == 1 && m.inputStep == 1:
			label = fmt.Sprintf("Model ID for %s: ", m.provider)
		case m.phase == 4:
			label = "Model ID: "
		}
		b.WriteString(promptLabelStyle.Render(label))
		b.WriteString(textInputStyle.Render(m.textInput))
		b.WriteString(lipgloss.NewStyle().
			Foreground(colorBrightAmber).
			Blink(true).
			Render("_"))
	}

	return b.String()
}

// ── Torus rendering (horn torus from horntorus.com) ────────────────────────────

func renderTorus(a, b float64) string {
	const (
		width  = 36
		height = 18

		// Canonical donut parameters (a1k0n.net)
		r1      = 1.0  // tube (minor) radius
		r2      = 2.0  // torus (major) radius
		k2      = 5.0  // distance from viewer
		thetaSp = 0.03 // theta step — finer = denser
		phiSp   = 0.01 // phi step — finer = denser
	)
	k1 := float64(width) * k2 * 3.0 / (8.0 * (r1 + r2))

	output := make([][]byte, height)
	zbuf := make([][]float64, height)
	for i := range output {
		output[i] = make([]byte, width)
		zbuf[i] = make([]float64, width)
		for j := range output[i] {
			output[i][j] = ' '
		}
	}

	chars := ".,-~:;=!*#$@"

	cosA, sinA := math.Cos(a), math.Sin(a)
	cosB, sinB := math.Cos(b), math.Sin(b)

	for theta := 0.0; theta < 6.28; theta += thetaSp {
		cosT, sinT := math.Cos(theta), math.Sin(theta)
		for phi := 0.0; phi < 6.28; phi += phiSp {
			cosP, sinP := math.Cos(phi), math.Sin(phi)

			cx := r2 + r1*cosT
			cy := r1 * sinT

			x3 := cx*(cosB*cosP+sinA*sinB*sinP) - cy*cosA*sinB
			y3 := cx*(sinB*cosP-sinA*cosB*sinP) + cy*cosA*cosB
			z := k2 + cosA*cx*sinP + cy*sinA
			ooz := 1.0 / z

			xp := int(float64(width)/2.0 + k1*ooz*x3)
			yp := int(float64(height)/2.0 - k1*ooz*y3*0.5)

			lum := cosP*cosT*sinB - cosA*cosT*sinP - sinA*sinT +
				cosB*(cosA*sinT-cosT*sinA*sinP)

			if yp >= 0 && yp < height && xp >= 0 && xp < width && ooz > zbuf[yp][xp] {
				zbuf[yp][xp] = ooz
				idx := int(lum * 8)
				if idx < 0 {
					idx = 0
				}
				if idx >= len(chars) {
					idx = len(chars) - 1
				}
				output[yp][xp] = chars[idx]
			}
		}
	}

	var sb strings.Builder
	for _, row := range output {
		sb.WriteString(string(row))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// colorTorus applies amber-shade lipgloss styles to each character based on
// its luminance bucket in the donut.c character ramp.
func colorTorus(frame string) string {
	var sb strings.Builder
	for _, ch := range frame {
		switch {
		case ch == '\n':
			sb.WriteByte('\n')
		case ch == ' ':
			sb.WriteByte(' ')
		case ch == '.' || ch == ',' || ch == '-':
			sb.WriteString(torusDimStyle.Render(string(ch)))
		case ch == '~' || ch == ':' || ch == ';':
			sb.WriteString(torusMidStyle.Render(string(ch)))
		case ch == '=' || ch == '!' || ch == '*':
			sb.WriteString(torusBrightStyle.Render(string(ch)))
		case ch == '#' || ch == '$' || ch == '@':
			sb.WriteString(torusMaxStyle.Render(string(ch)))
		default:
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// ── Public entry point ────────────────────────────────────────────────────────

// RunStartup shows an interactive provider/model selection menu.
// Returns a SetupResult with provider, model, and optional config overrides.
// If skipStartup is true, returns an empty SetupResult.
func RunStartup(skipStartup bool) SetupResult {
	if skipStartup {
		return SetupResult{}
	}

	m := newSetupModel()

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return SetupResult{}
	}

	result := finalModel.(setupModel)
	if !result.done {
		return SetupResult{}
	}

	return SetupResult{
		Provider: result.provider,
		Model:    result.model,
		Config:   result.configOverrides,
	}
}
