package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"torus_go_agent/internal/config"

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
	freeChoices := []ModelChoice{
		{Name: "Free Models Router [Free]", ID: "nvidia/free", ContextWindow: 131072, MaxTokens: 8192},
	}
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
	CompactionModel        string
	ContinuousCompression  bool
	CompressionKeepFirst   int
	CompressionKeepLast    int
	CompressionMinMessages int
	ZoneBudgeting          bool
	ZoneArchivePercent     int
	SmartRouting           bool
	SmartRoutingModel      string
	SteeringMode           string
	PersistThinking        bool
	Thinking               string
	ThinkingBudget         int
}

func defaultOverrides() *AgentConfigOverrides {
	return overridesFromConfig(config.DefaultAgentConfig())
}

// overridesFromConfig creates config overrides from an AgentConfig.
// MaxTokens and ContextWindow are set to 0 ("default") to allow auto-resolution.
func overridesFromConfig(a config.AgentConfig) *AgentConfigOverrides {
	mode := a.SteeringMode
	if mode == "" {
		mode = "mild"
	}
	return &AgentConfigOverrides{
		MaxTokens:              0, // 0 = "default" (auto-resolved from OpenRouter/models.json)
		ContextWindow:          0, // 0 = "default" (auto-resolved from OpenRouter/models.json)
		Compaction:             a.Compaction,
		CompactionTrigger:      a.CompactionTrigger,
		CompactionThreshold:    a.CompactionThreshold,
		CompactionMaxMessages:  a.CompactionMaxMessages,
		CompactionKeepLastN:    a.CompactionKeepLastN,
		CompactionModel:        a.CompactionModel,
		ContinuousCompression:  a.ContinuousCompression,
		CompressionKeepFirst:   a.CompressionKeepFirst,
		CompressionKeepLast:    a.CompressionKeepLast,
		CompressionMinMessages: a.CompressionMinMessages,
		ZoneBudgeting:          a.ZoneBudgeting,
		ZoneArchivePercent:     a.ZoneArchivePercent,
		SmartRouting:           a.SmartRouting,
		SmartRoutingModel:      a.SmartRoutingModel,
		SteeringMode:           mode,
		PersistThinking:        a.PersistThinking,
		Thinking:               a.Thinking,
		ThinkingBudget:         a.ThinkingBudget,
	}
}

// savedOverrides returns config overrides from savedConfig if available, otherwise code defaults.
func (m setupModel) savedOverrides() *AgentConfigOverrides {
	if m.savedConfig != nil {
		return overridesFromConfig(*m.savedConfig)
	}
	return defaultOverrides()
}

// resolveModelSpecs is kept for future use but MaxTokens/ContextWindow
// are now resolved in main.go after the startup screen, so the user can
// override them. The settings screen shows "default" for these fields.
func (m *setupModel) resolveModelSpecs() {
	// No-op: MaxTokens/ContextWindow resolved in main.go
}

type configField struct {
	name    string
	kind    string   // "bool", "int", "string", "cycle"
	options []string // for "cycle" kind only
}

var configFields = []configField{
	{"Compaction", "cycle", []string{"llm", "sliding", "off"}},            // 0
	{"CompactionTrigger", "cycle", []string{"both", "tokens", "messages"}},// 1
	{"CompactionThreshold", "int", nil},    // 2
	{"CompactionMaxMessages", "int", nil},  // 3
	{"CompactionKeepLastN", "int", nil},    // 4
	{"CompactionModel", "string", nil},     // 5
	{"ContinuousCompression", "bool", nil}, // 6
	{"CompressionKeepFirst", "int", nil},   // 7
	{"CompressionKeepLast", "int", nil},    // 8
	{"CompressionMinMessages", "int", nil}, // 9
	{"ZoneBudgeting", "bool", nil},         // 10
	{"ZoneArchivePercent", "int", nil},     // 11
	{"SmartRouting", "bool", nil},          // 12
	{"SmartRoutingModel", "string", nil},   // 13
	{"SteeringMode", "cycle", []string{"mild", "aggressive"}}, // 14
	{"PersistThinking", "bool", nil},       // 15
	{"Thinking", "cycle", []string{"", "low", "mid", "high", "max", "ultra"}}, // 16
	{"ThinkingBudget", "int", nil},         // 17
	{"MaxTokens", "int", nil},              // 18
	{"ContextWindow", "int", nil},          // 19
}

func (o *AgentConfigOverrides) getValue(idx int) string {
	switch idx {
	case 0:
		return o.Compaction
	case 1:
		return o.CompactionTrigger
	case 2:
		return fmt.Sprintf("%d", o.CompactionThreshold)
	case 3:
		return fmt.Sprintf("%d", o.CompactionMaxMessages)
	case 4:
		return fmt.Sprintf("%d", o.CompactionKeepLastN)
	case 5:
		if o.CompactionModel == "" {
			return "(main model)"
		}
		return formatProviderModel(o.CompactionModel)
	case 6:
		if o.ContinuousCompression {
			return "true"
		}
		return "false"
	case 7:
		return fmt.Sprintf("%d", o.CompressionKeepFirst)
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
		if o.SmartRoutingModel == "" {
			return "(none)"
		}
		return formatProviderModel(o.SmartRoutingModel)
	case 14:
		return o.SteeringMode
	case 15:
		if o.PersistThinking {
			return "true"
		}
		return "false"
	case 16:
		if o.Thinking == "" {
			return "off"
		}
		return o.Thinking
	case 17:
		if o.ThinkingBudget == 0 {
			return "auto"
		}
		return fmt.Sprintf("%d", o.ThinkingBudget)
	case 18:
		if o.MaxTokens == 0 {
			return "default"
		}
		return fmt.Sprintf("%d", o.MaxTokens)
	case 19:
		if o.ContextWindow == 0 {
			return "default"
		}
		return fmt.Sprintf("%d", o.ContextWindow)
	}
	return ""
}

func (o *AgentConfigOverrides) setValue(idx int, val string) {
	n, _ := strconv.Atoi(val)
	switch idx {
	case 0:
		o.Compaction = val
	case 1:
		o.CompactionTrigger = val
	case 2:
		o.CompactionThreshold = n
	case 3:
		o.CompactionMaxMessages = n
	case 4:
		o.CompactionKeepLastN = n
	case 5:
		o.CompactionModel = val
	case 6:
		o.ContinuousCompression = val == "true"
	case 7:
		o.CompressionKeepFirst = n
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
		o.SmartRoutingModel = val
	case 14:
		o.SteeringMode = val
	case 15:
		o.PersistThinking = val == "true"
	case 16:
		o.Thinking = val
	case 17:
		o.ThinkingBudget = n
	case 18:
		o.MaxTokens = n
	case 19:
		o.ContextWindow = n
	}
}

func (o *AgentConfigOverrides) toggleBool(idx int) {
	switch idx {
	case 6:
		o.ContinuousCompression = !o.ContinuousCompression
	case 10:
		o.ZoneBudgeting = !o.ZoneBudgeting
	case 12:
		o.SmartRouting = !o.SmartRouting
	case 15:
		o.PersistThinking = !o.PersistThinking
	}
}

// cycleOption cycles through the options for a cycle field.
// dir: +1 = right/forward, -1 = left/backward
func (o *AgentConfigOverrides) cycleOption(idx, dir int) {
	f := configFields[idx]
	if f.kind != "cycle" || len(f.options) == 0 {
		return
	}
	current := o.getValue(idx)
	pos := 0
	for i, opt := range f.options {
		if opt == current {
			pos = i
			break
		}
	}
	pos = (pos + dir + len(f.options)) % len(f.options)
	o.setValue(idx, f.options[pos])
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

// modelPickerEntry is a flattened model entry with provider context.
type modelPickerEntry struct {
	Label       string // e.g. "Claude Opus 4.6 (Anthropic)"
	ModelID     string // e.g. "claude-opus-4-6"
	ProviderKey string // e.g. "anthropic"
}

// formatProviderModel formats "provider:model" as "model (provider)" for display.
func formatProviderModel(pm string) string {
	if idx := strings.IndexByte(pm, ':'); idx > 0 {
		return pm[idx+1:] + " (" + pm[:idx] + ")"
	}
	return pm
}

// ProviderModelID returns "provider:model" format for storage in config.
func (e modelPickerEntry) ProviderModelID() string {
	if e.ModelID == "" {
		return ""
	}
	return e.ProviderKey + ":" + e.ModelID
}

// buildModelPickerItems gathers all models from all provider groups into a flat list.
func buildModelPickerItems(groups []ProviderGroup) []modelPickerEntry {
	var items []modelPickerEntry
	// First entry: clear/none
	items = append(items, modelPickerEntry{Label: "(none — use main model)", ModelID: "", ProviderKey: ""})
	for _, g := range groups {
		provName := g.Name
		if g.ProviderKey == "" {
			continue // skip custom provider
		}
		// Direct models
		for _, mc := range g.Models {
			if mc.ID == "" {
				continue // skip "Custom model ID" entries
			}
			items = append(items, modelPickerEntry{
				Label:       mc.Name + " (" + provName + ")",
				ModelID:     mc.ID,
				ProviderKey: g.ProviderKey,
			})
		}
		// Category models
		for _, cat := range g.Categories {
			for _, mc := range cat.Models {
				if mc.ID == "" {
					continue
				}
				items = append(items, modelPickerEntry{
					Label:       mc.Name + " (" + provName + " / " + cat.Name + ")",
					ModelID:     mc.ID,
					ProviderKey: g.ProviderKey,
				})
			}
		}
	}
	return items
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
					{Name: "Free Models Router [Free]", ID: "nvidia/free", ContextWindow: 131072, MaxTokens: 8192},
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
				{Name: "Claude Opus 4.6", ID: "claude-opus-4-6"},
				{Name: "Claude Sonnet 4.6", ID: "claude-sonnet-4-6"},
				{Name: "Claude Haiku 4.5", ID: "claude-haiku-4-5-20251001"},
				{Name: "Claude Sonnet 4.5", ID: "claude-sonnet-4-5-20250929"},
				{Name: "Claude Opus 4.5", ID: "claude-opus-4-5-20251101"},
				{Name: "Claude Opus 4.1", ID: "claude-opus-4-1-20250805"},
				{Name: "Claude Sonnet 4", ID: "claude-sonnet-4-20250514"},
				{Name: "Claude Opus 4", ID: "claude-opus-4-20250514"},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name: "OpenAI", ProviderKey: "openai",
			AuthMethods: []AuthMethod{{Name: "API key", NeedsKey: "OPENAI_API_KEY"}},
			Models: []ModelChoice{
				{Name: "GPT-5.4", ID: "gpt-5.4"},
				{Name: "GPT-5.4 Mini", ID: "gpt-5.4-mini"},
				{Name: "GPT-5.4 Nano", ID: "gpt-5.4-nano"},
				{Name: "GPT-4.1", ID: "gpt-4.1"},
				{Name: "GPT-4.1 Mini", ID: "gpt-4.1-mini"},
				{Name: "GPT-4.1 Nano", ID: "gpt-4.1-nano"},
				{Name: "GPT-4o", ID: "gpt-4o"},
				{Name: "GPT-4o Mini", ID: "gpt-4o-mini"},
				{Name: "o4-mini", ID: "o4-mini"},
				{Name: "o3", ID: "o3"},
				{Name: "o3 Mini", ID: "o3-mini"},
				{Name: "o3 Pro", ID: "o3-pro"},
				{Name: "o1", ID: "o1"},
				{Name: "o1 Mini", ID: "o1-mini"},
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
				{Name: "grok-3-mini", ID: "grok-3-mini", ContextWindow: 131072, MaxTokens: 131072},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name: "Google Gemini", ProviderKey: "gemini",
			AuthMethods: []AuthMethod{{Name: "API key", NeedsKey: "GEMINI_API_KEY"}},
			Models: []ModelChoice{
				{Name: "Gemini 3.1 Pro (preview)", ID: "gemini-3.1-pro-preview"},
				{Name: "Gemini 3.1 Flash Lite (preview) [Free]", ID: "gemini-3.1-flash-lite-preview"},
				{Name: "Gemini 3 Flash (preview) [Free]", ID: "gemini-3-flash-preview"},
				{Name: "Gemini 2.5 Pro", ID: "gemini-2.5-pro"},
				{Name: "Gemini 2.5 Flash [Free]", ID: "gemini-2.5-flash"},
				{Name: "Gemini 2.5 Flash Lite [Free]", ID: "gemini-2.5-flash-lite"},
				{Name: "Gemini 2.0 Flash [Free]", ID: "gemini-2.0-flash"},
				{Name: "Gemini 2.0 Flash Lite [Free]", ID: "gemini-2.0-flash-lite"},
				{Name: "Custom model ID", ID: ""},
			},
		},
		{
			Name: "Azure OpenAI", ProviderKey: "azure",
			AuthMethods: []AuthMethod{{Name: "API key", NeedsKey: "AZURE_OPENAI_API_KEY"}},
			Models: []ModelChoice{
				{Name: "GPT-5.4", ID: "gpt-5.4"},
				{Name: "GPT-5.4 Mini", ID: "gpt-5.4-mini"},
				{Name: "GPT-4.1", ID: "gpt-4.1"},
				{Name: "GPT-4.1 Mini", ID: "gpt-4.1-mini"},
				{Name: "GPT-4o", ID: "gpt-4o"},
				{Name: "o4-mini", ID: "o4-mini"},
				{Name: "o3", ID: "o3"},
				{Name: "o3 Mini", ID: "o3-mini"},
				{Name: "Custom deployment", ID: ""},
			},
		},
		{
			Name: "Vertex AI", ProviderKey: "vertex",
			AuthMethods: []AuthMethod{{Name: "Access token", NeedsKey: "VERTEX_ACCESS_TOKEN"}},
			Models: []ModelChoice{
				{Name: "Gemini 3.1 Pro (preview)", ID: "gemini-3.1-pro-preview"},
				{Name: "Gemini 3 Flash (preview)", ID: "gemini-3-flash-preview"},
				{Name: "Gemini 2.5 Pro", ID: "gemini-2.5-pro"},
				{Name: "Gemini 2.5 Flash", ID: "gemini-2.5-flash"},
				{Name: "Gemini 2.5 Flash Lite", ID: "gemini-2.5-flash-lite"},
				{Name: "Gemini 2.0 Flash", ID: "gemini-2.0-flash"},
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

	// Torus depth gradient — expanded orange palette for 3D illusion.
	// Indexed by continuous brightness value (0.0–1.0+).
	torusDepthGradient = []lipgloss.Color{
		lipgloss.Color("#331100"), // deep shadow
		lipgloss.Color("#552200"), // dark ember
		lipgloss.Color("#7a2e00"), // brown-orange
		lipgloss.Color("#993300"), // dim ember
		lipgloss.Color("#bb3a00"), // warm dark
		lipgloss.Color("#cc4400"), // warm orange
		lipgloss.Color("#dd5500"), // medium orange
		lipgloss.Color("#ff4d01"), // neon orange
		lipgloss.Color("#ff6600"), // hot orange
		lipgloss.Color("#ff8800"), // bright orange
		lipgloss.Color("#ffaa00"), // gold
		lipgloss.Color("#ffcc44"), // bright gold highlight
	}

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

const asciiTitle = "" +
	"::::::::::::    ...     :::::::..    ...    ::: .::::::.                   \n" +
	";;;;;;;;'''' .;;;;;;;.  ;;;;``;;;;   ;;     ;;;;;;`    `                   \n" +
	"     [[     ,[[     \\[[, [[[,/[[['  [['     [[['[==/[[[[,                  \n" +
	"     $$     $$$,     $$$ $$$$$$c    $$      $$$  '''    $                  \n" +
	"     88,    \"888,_ _,88P 888b \"88bo,88    .d888 88b    dP                  \n" +
	"     MMM      \"YMMMMMP\"  MMMM   \"W\"  \"YmmMMMM\"\"  \"YMmMY\"                  \n" +
	"                                                                          \n" +
	"                                                                          \n" +
	"  .,-:::::/               :::.                                    `::     \n" +
	",;;-'````'                ;;`;;                                    ;;     \n" +
	"[[[   [[[[[[/ ,ccc,      ,[[ '[[,    ,ccc,   ,cc[[[cc. [ccccc,  =[[[[[[.\n" +
	"\"$$c.    \"$$ $$$\"c$$$   c$$$cc$$$c  $$$cc$$$ $$$___--' $$$$\"$$$    $$     \n" +
	" `Y8bo,,,o88o888   88    888   888  888   88888b    ,o,888  Y88o   88,   \n" +
	"   `'YMUP\"YMM \"YUMMP     YMM   \\\"\\\"` \"YUM\" MP \"YUMMMMP\"MMM  \"MMM   MMM   \n" +
	"                                          MMM                            \n" +
	"                                    ,c.   ###                            \n" +
	"                                    \\M###MMU                             \n" +
	"                                     'YMUP\"                              "

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
	// Find max line width so all lines pad to the same width (uniform centering).
	maxW := 0
	for _, line := range lines {
		if w := len([]rune(line)); w > maxW {
			maxW = w
		}
	}
	var sb strings.Builder
	for _, line := range lines {
		runes := []rune(line)
		for len(runes) < maxW {
			runes = append(runes, ' ')
		}
		for col, ch := range runes {
			if ch == ' ' {
				sb.WriteByte(' ')
				continue
			}
			// Wave: shift color index by column + phase
			idx := (col + int(phase*10)) % gradLen
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
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ── Setup model ───────────────────────────────────────────────────────────────

type setupModel struct {
	width, height int
	ready         bool

	// Splash screen
	showSplash bool

	// Torus animation
	torusA, torusB float64
	torusFrame     string
	particles      []torusParticle

	// Menu phases:
	// 0=main, 1=pick provider, 2=pick auth, 3=pick category (if any), 4=pick model,
	// 5=config mode, 6=edit settings, 7=model picker (for compactionModel/smartRoutingModel)
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

	// Type-to-filter for list screens (phases 1, 3, 4)
	filterText string

	// Config customization (phase 5 = choose config mode, phase 6 = edit settings)
	savedConfig     *config.AgentConfig   // loaded from config.json, used to pre-fill settings
	configOverrides *AgentConfigOverrides
	editingConfig   bool   // true when editing a numeric/string value
	editBuffer      string // text buffer for numeric/string input

	// Model picker (phase 7) — flat list for compactionModel/smartRoutingModel
	modelPickerItems []modelPickerEntry // all models from all providers
	modelPickerField int                // config field index that triggered the picker

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

	m := setupModel{groups: groups, showSplash: true}
	m.particles = initTorusParticles()
	m.torusFrame = renderParticleTorus(m.particles, m.torusB, m.torusA)
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
		m.torusA += 0.032
		m.torusB += 0.02
		updateTorusParticles(m.particles, m.torusA)
		m.torusFrame = renderParticleTorus(m.particles, m.torusB, m.torusA)
		return m, startupTickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m setupModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ── Splash screen: enter to continue ─────────────────────────
	if m.showSplash {
		switch msg.String() {
		case "enter", " ":
			m.showSplash = false
		case "ctrl+c", "q":
			m.done = true
			return m, tea.Quit
		}
		return m, nil
	}

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

	case "ctrl+c":
		m.provider = ""
		m.model = ""
		m.done = true
		return m, tea.Quit

	case "q":
		if m.filterablePhase() {
			m.filterText += "q"
			m.cursor = 0
			m.scrollOffset = 0
		} else {
			m.provider = ""
			m.model = ""
			m.done = true
			return m, tea.Quit
		}

	case "up":
		total := m.menuLen()
		if m.cursor > 0 {
			m.cursor--
		} else {
			m.cursor = total - 1
		}
		m.scrollOffset = clampScrollOffset(m.cursor, m.scrollOffset, total)

	case "k":
		if !m.filterablePhase() {
			total := m.menuLen()
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = total - 1
			}
			m.scrollOffset = clampScrollOffset(m.cursor, m.scrollOffset, total)
		} else {
			m.filterText += "k"
			m.cursor = 0
			m.scrollOffset = 0
		}

	case "down":
		total := m.menuLen()
		if m.cursor < total-1 {
			m.cursor++
		} else {
			m.cursor = 0
		}
		m.scrollOffset = clampScrollOffset(m.cursor, m.scrollOffset, total)

	case "j":
		if !m.filterablePhase() {
			total := m.menuLen()
			if m.cursor < total-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
			m.scrollOffset = clampScrollOffset(m.cursor, m.scrollOffset, total)
		} else {
			m.filterText += "j"
			m.cursor = 0
			m.scrollOffset = 0
		}

	case "enter", " ":
		return m.selectItem()

	case "left":
		if m.phase == 6 && m.cursor < len(configFields) {
			f := configFields[m.cursor]
			if f.kind == "cycle" {
				m.configOverrides.cycleOption(m.cursor, -1)
			} else if f.kind == "bool" {
				m.configOverrides.toggleBool(m.cursor)
			}
		}

	case "h":
		if !m.filterablePhase() {
			if m.phase == 6 && m.cursor < len(configFields) {
				f := configFields[m.cursor]
				if f.kind == "cycle" {
					m.configOverrides.cycleOption(m.cursor, -1)
				} else if f.kind == "bool" {
					m.configOverrides.toggleBool(m.cursor)
				}
			}
		} else {
			m.filterText += "h"
			m.cursor = 0
			m.scrollOffset = 0
		}

	case "right":
		if m.phase == 6 && m.cursor < len(configFields) {
			f := configFields[m.cursor]
			if f.kind == "cycle" {
				m.configOverrides.cycleOption(m.cursor, 1)
			} else if f.kind == "bool" {
				m.configOverrides.toggleBool(m.cursor)
			}
		}

	case "l":
		if !m.filterablePhase() {
			if m.phase == 6 && m.cursor < len(configFields) {
				f := configFields[m.cursor]
				if f.kind == "cycle" {
					m.configOverrides.cycleOption(m.cursor, 1)
				} else if f.kind == "bool" {
					m.configOverrides.toggleBool(m.cursor)
				}
			}
		} else {
			m.filterText += "l"
			m.cursor = 0
			m.scrollOffset = 0
		}

	case "backspace":
		if m.filterText != "" {
			m.filterText = m.filterText[:len(m.filterText)-1]
			m.cursor = 0
			m.scrollOffset = 0
		}

	case "esc":
		// Clear filter first; if no filter, navigate back
		if m.filterText != "" {
			m.filterText = ""
			m.cursor = 0
			m.scrollOffset = 0
			return m, nil
		}
		switch m.phase {
		case 7: // model picker → settings
			m.phase = 6
			m.cursor = m.modelPickerField
			m.scrollOffset = 0
			m.modelPickerItems = nil
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

	default:
		// Type-to-filter on list screens
		if m.filterablePhase() {
			r := msg.String()
			if len(r) == 1 && r[0] >= 32 && r[0] <= 126 {
				m.filterText += r
				m.cursor = 0
				m.scrollOffset = 0
			}
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
			m.configOverrides = m.savedOverrides()
			m.resolveModelSpecs()
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
			m.configOverrides = m.savedOverrides()
			m.resolveModelSpecs()
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

// filteredIndices returns the indices of items whose label contains the filter text.
// If filterText is empty, returns all indices.
func filteredIndices(total int, labelFn func(i int) string, filter string) []int {
	if filter == "" {
		idx := make([]int, total)
		for i := range idx {
			idx[i] = i
		}
		return idx
	}
	lower := strings.ToLower(filter)
	var idx []int
	for i := 0; i < total; i++ {
		if strings.Contains(strings.ToLower(labelFn(i)), lower) {
			idx = append(idx, i)
		}
	}
	return idx
}

// filterablePhase returns true if the current phase supports type-to-filter.
func (m setupModel) filterablePhase() bool {
	return m.phase == 1 || m.phase == 3 || m.phase == 4 || m.phase == 7
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
	if m.filterText != "" && m.filterablePhase() {
		return len(m.filteredItems())
	}
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
	case 7:
		return len(m.modelPickerItems)
	}
	return 0
}

// filteredItems returns filtered indices for the current phase.
func (m setupModel) filteredItems() []int {
	switch m.phase {
	case 1:
		return filteredIndices(len(m.groups), func(i int) string {
			return m.groups[i].Name
		}, m.filterText)
	case 3:
		if m.selectedGroup != nil {
			return filteredIndices(len(m.selectedGroup.Categories), func(i int) string {
				return m.selectedGroup.Categories[i].Name
			}, m.filterText)
		}
	case 4:
		models := m.currentModels()
		return filteredIndices(len(models), func(i int) string {
			return models[i].Name
		}, m.filterText)
	case 7:
		return filteredIndices(len(m.modelPickerItems), func(i int) string {
			return m.modelPickerItems[i].Label
		}, m.filterText)
	}
	return nil
}

// resolveFilteredIndex maps cursor position to original index when filtering.
func (m setupModel) resolveFilteredIndex() int {
	if m.filterText == "" || !m.filterablePhase() {
		return m.cursor
	}
	items := m.filteredItems()
	if m.cursor < len(items) {
		return items[m.cursor]
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
		idx := m.resolveFilteredIndex()
		m.filterText = ""
		group := m.groups[idx]
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
		idx := m.resolveFilteredIndex()
		m.filterText = ""
		cat := m.selectedGroup.Categories[idx]
		m.selectedCategory = &cat
		m.phase = 4
		m.cursor = 0
		m.scrollOffset = 0
		return m, nil

	case 4: // Pick model
		idx := m.resolveFilteredIndex()
		m.filterText = ""
		models := m.currentModels()
		mc := models[idx]
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
		m.configOverrides = m.savedOverrides()
		if mc.ContextWindow > 0 {
			m.configOverrides.ContextWindow = mc.ContextWindow
			m.configOverrides.MaxTokens = mc.MaxTokens
		} else {
			m.resolveModelSpecs()
		}
		return m, nil

	case 5: // Config mode
		switch m.cursor {
		case 0:
			m.configOverrides = m.savedOverrides()
			m.done = true
			return m, tea.Quit
		case 1:
			m.configOverrides = nil
			m.done = true
			return m, tea.Quit
		case 2:
			if m.configOverrides == nil {
				m.configOverrides = m.savedOverrides()
			}
			m.phase = 6
			m.cursor = 0
			m.scrollOffset = 0
		}

	case 6: // Edit config
		if m.cursor >= len(configFields) {
			m.done = true
			return m, tea.Quit
		}
		f := configFields[m.cursor]
		// CompactionModel (5) and SmartRoutingModel (12) open model picker
		if m.cursor == 5 || m.cursor == 12 {
			m.modelPickerItems = buildModelPickerItems(m.groups)
			m.modelPickerField = m.cursor
			m.phase = 7
			m.filterText = ""
			m.cursor = 0
			m.scrollOffset = 0
			return m, nil
		}
		switch f.kind {
		case "bool":
			m.configOverrides.toggleBool(m.cursor)
		case "cycle":
			m.configOverrides.cycleOption(m.cursor, 1)
		default:
			m.editingConfig = true
			m.editBuffer = m.configOverrides.getValue(m.cursor)
		}

	case 7: // Model picker
		idx := m.resolveFilteredIndex()
		m.filterText = ""
		if idx < len(m.modelPickerItems) {
			entry := m.modelPickerItems[idx]
			m.configOverrides.setValue(m.modelPickerField, entry.ProviderModelID())
		}
		m.cursor = m.modelPickerField
		m.phase = 6
		m.scrollOffset = 0
		m.modelPickerItems = nil
	}

	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m setupModel) View() string {
	if !m.ready {
		return ""
	}

	var b strings.Builder

	// ── Splash vs Setup ────────────────────────────────────────────
	if m.showSplash {
		torusPanel := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, m.torusFrame)
		b.WriteString(torusPanel)
		subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4d01")).Render("press enter to continue")
		b.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, subtitle))
		return b.String()
	}

	// ── Setup menu page ─────────────────────────────────────────────
	titleRendered := renderAnimatedTitle(asciiTitle, m.torusA)
	titleBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, titleRendered)
	b.WriteString(titleBlock)
	b.WriteString("\n\n")

	menuContent := m.renderMenu()
	menuPanel := menuPanelStyle.Render(menuContent)
	centered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, menuPanel)
	b.WriteString(centered)

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
		if m.filterText != "" {
			b.WriteString(textInputStyle.Render("  / " + m.filterText + "_"))
			b.WriteByte('\n')
		}
		indices := m.filteredItems()
		m.renderScrollableItems(&b, len(indices), func(i int) string {
			return m.groups[indices[i]].Name
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
		if m.filterText != "" {
			b.WriteString(textInputStyle.Render("  / " + m.filterText + "_"))
			b.WriteByte('\n')
		}
		if m.selectedGroup != nil {
			cats := m.selectedGroup.Categories
			indices := m.filteredItems()
			m.renderScrollableItems(&b, len(indices), func(i int) string {
				return fmt.Sprintf("%s (%d)", cats[indices[i]].Name, len(cats[indices[i]].Models))
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
		if m.filterText != "" {
			b.WriteString(textInputStyle.Render("  / " + m.filterText + "_"))
			b.WriteByte('\n')
		}
		models := m.currentModels()
		indices := m.filteredItems()
		m.renderScrollableItems(&b, len(indices), func(i int) string {
			return models[indices[i]].Name
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

	case 7: // Model picker
		fieldName := configFields[m.modelPickerField].name
		b.WriteString(menuHeaderStyle.Render("Select " + fieldName))
		b.WriteByte('\n')
		if m.filterText != "" {
			b.WriteString(textInputStyle.Render("  / " + m.filterText + "_"))
			b.WriteByte('\n')
		}
		indices := m.filteredItems()
		m.renderScrollableItems(&b, len(indices), func(i int) string {
			return m.modelPickerItems[indices[i]].Label
		})
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

// ── Particle-based torus rendering ─────────────────────────────────────────────

const torusWidth = 120
const torusHeight = 50
const numTorusParticles = 450

// Flowing particle color palette (dim, atmospheric — converge to orange).
var flowParticleStyles = []lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("#336699")), // blue
	lipgloss.NewStyle().Foreground(lipgloss.Color("#339966")), // green
	lipgloss.NewStyle().Foreground(lipgloss.Color("#993366")), // magenta
	lipgloss.NewStyle().Foreground(lipgloss.Color("#996633")), // amber
	lipgloss.NewStyle().Foreground(lipgloss.Color("#339999")), // cyan
	lipgloss.NewStyle().Foreground(lipgloss.Color("#663399")), // purple
}

type torusParticle struct {
	x, y, z       float64
	speed         float64
	phase         float64
	flowDirection float64
	targetAngle   float64
	spiralRadius  float64
	settled       bool
	torusAngle    float64
	torusProgress float64
	brightness    float64
}

type torusVector3 struct {
	x, y, z float64
}

func resetTorusParticle(p *torusParticle) {
	angle := rand.Float64() * math.Pi * 2
	radius := rand.Float64()*15 + 10
	fromAbove := rand.Float64() > 0.5

	p.x = math.Cos(angle) * radius
	if fromAbove {
		p.y = rand.Float64()*30 + 25
	} else {
		p.y = -(rand.Float64()*30 + 25)
	}
	p.z = math.Sin(angle) * radius

	p.speed = rand.Float64()*0.3 + 0.2
	p.phase = rand.Float64() * math.Pi * 2
	if fromAbove {
		p.flowDirection = -1
	} else {
		p.flowDirection = 1
	}
	p.targetAngle = angle
	p.spiralRadius = radius * 0.9
	p.settled = false
	p.torusAngle = rand.Float64() * math.Pi * 2
	p.torusProgress = 0
	p.brightness = 0
}

func initTorusParticles() []torusParticle {
	particles := make([]torusParticle, numTorusParticles)
	for i := range particles {
		resetTorusParticle(&particles[i])
	}
	return particles
}

func getTorusPosition(p *torusParticle, t float64) torusVector3 {
	radius := 6.0
	tube := 6.0

	revolutionSpeed := t * 1.5
	revolvedAngle := p.torusAngle + revolutionSpeed

	overallRotation := t * 0.5

	torusY := math.Sin(revolvedAngle) * tube
	torusRadius := radius + math.Cos(revolvedAngle)*tube

	return torusVector3{
		x: math.Cos(p.torusProgress+overallRotation) * torusRadius,
		y: torusY,
		z: math.Sin(p.torusProgress+overallRotation) * torusRadius,
	}
}

func updateTorusParticles(particles []torusParticle, t float64) {
	for i := range particles {
		p := &particles[i]
		if !p.settled {
			distanceToCenter := math.Abs(p.y)

			if distanceToCenter < 20 {
				spiralIntensity := 1 - (distanceToCenter / 20)
				p.phase += 0.08

				currentRadius := p.spiralRadius * (1 - spiralIntensity*0.85)
				p.x = math.Cos(p.targetAngle+p.phase) * currentRadius
				p.z = math.Sin(p.targetAngle+p.phase) * currentRadius
				p.y += p.speed * p.flowDirection

				p.brightness = math.Min(p.brightness+0.1, 1)
			} else {
				p.y += p.speed * p.flowDirection
				p.brightness = math.Min(p.brightness+0.05, 0.6)
			}

			if distanceToCenter < 8 && rand.Float64() < 0.4 {
				p.settled = true
				p.torusProgress = rand.Float64() * math.Pi * 2
			}

			if math.Abs(p.y) > 50 {
				resetTorusParticle(p)
			}
		} else {
			p.phase += 0.02
			pos := getTorusPosition(p, t)

			p.x += (pos.x - p.x) * 0.15
			p.y += (pos.y - p.y) * 0.15
			p.z += (pos.z - p.z) * 0.15

			p.brightness = 0.7 + math.Sin(t*8+p.phase*10)*0.3

			if rand.Float64() < 0.0002 {
				resetTorusParticle(p)
			}
		}
	}
}

type torusProjectedPoint struct {
	x, y       int
	z          float64
	brightness float64
}

func project3DTo2D(x, y, z, rotationAngle, timeValue float64) torusProjectedPoint {
	cosAngle := math.Cos(rotationAngle)
	sinAngle := math.Sin(rotationAngle)

	rotatedX := x*cosAngle - z*sinAngle
	rotatedZ := x*sinAngle + z*cosAngle

	tiltAngle := math.Sin(timeValue*0.3) * 0.2
	cosTilt := math.Cos(tiltAngle)
	sinTilt := math.Sin(tiltAngle)

	tiltedY := y*cosTilt - rotatedZ*sinTilt
	tiltedZ := y*sinTilt + rotatedZ*cosTilt

	scale := 200 / (200 + tiltedZ)

	return torusProjectedPoint{
		x:          int(rotatedX*scale*2.5 + torusWidth/2),
		y:          int(tiltedY*scale*1.2+torusHeight/2) + 7,
		z:          tiltedZ,
		brightness: scale,
	}
}

type torusCell struct {
	ch         rune
	depth      float64
	brightness float64
	settled    bool
	colorI     int // >=0 = particle color index, <0 = title char (-(gIdx+1))
}

func renderParticleTorus(particles []torusParticle, _ float64, t float64) string {
	// Build grid
	grid := make([][]torusCell, torusHeight)
	for i := range grid {
		grid[i] = make([]torusCell, torusWidth)
		for j := range grid[i] {
			grid[i][j] = torusCell{ch: ' ', depth: math.Inf(-1)}
		}
	}

	rotationAngle := t * 0.15

	// Project particles into grid
	for pi, p := range particles {
		proj := project3DTo2D(p.x, p.y, p.z, rotationAngle, t)

		if proj.x >= 0 && proj.x < torusWidth && proj.y >= 0 && proj.y < torusHeight {
			if proj.z > grid[proj.y][proj.x].depth {
				brightness := p.brightness * proj.brightness

				var ch rune
				colorIdx := pi % len(flowParticleStyles)
				if p.settled {
					// Brightness-based small symbols for settled particles
					if brightness > 0.8 {
						ch = '*'
					} else if brightness > 0.6 {
						ch = '~'
					} else if brightness > 0.4 {
						ch = ':'
					} else {
						ch = '.'
					}
				} else {
					if brightness > 0.7 {
						ch = '*'
					} else if brightness > 0.4 {
						ch = 'o'
					} else {
						ch = '.'
					}
				}

				grid[proj.y][proj.x] = torusCell{
					ch:         ch,
					depth:      proj.z,
					brightness: brightness,
					settled:    p.settled,
					colorI:     colorIdx,
				}
			}
		}
	}

	// Stamp title overlay into the grid
	titleLines := strings.Split(asciiTitle, "\n")
	// Find max title width
	maxTW := 0
	for _, line := range titleLines {
		if w := len([]rune(line)); w > maxTW {
			maxTW = w
		}
	}
	ty := 0 // title starts at top
	offsetY := ty + 4
	offsetX := (torusWidth - maxTW) / 2
	gradLen := len(titleGradient)
	for li, line := range titleLines {
		runes := []rune(line)
		for ci, ch := range runes {
			if ch == ' ' {
				continue
			}
			gy := offsetY + li
			gx := offsetX + ci
			if gy >= 0 && gy < torusHeight && gx >= 0 && gx < torusWidth {
				// Wave effect: shift color index by column + phase
				gIdx := (ci + int(t*10)) % gradLen
				if gIdx < 0 {
					gIdx += gradLen
				}
				grid[gy][gx] = torusCell{
					ch:     ch,
					depth:  math.Inf(1), // title always on top
					colorI: -(gIdx + 1), // negative = title char
				}
			}
		}
	}


	// Stamp pulsing "press enter" subtitle below the torus
	pressEnter := "▸ press enter to continue ◂"
	peX := (torusWidth - len([]rune(pressEnter))) / 2
	peY := torusHeight - 4
	gradLen = len(titleGradient)
	pulseIdx := int(t*8) % gradLen
	if pulseIdx < 0 { pulseIdx += gradLen }
	for ci, ch := range pressEnter {
		gx := peX + ci
		if gx >= 0 && gx < torusWidth && peY >= 0 && peY < torusHeight {
			grid[peY][gx] = torusCell{
				ch:     ch,
				depth:  math.Inf(1),
				colorI: -(pulseIdx + 1),
			}
		}
	}
	// Render grid to string with colors
	var sb strings.Builder
	for _, row := range grid {
		for _, cell := range row {
			if cell.ch == ' ' {
				sb.WriteByte(' ')
				continue
			}
			if cell.colorI < 0 {
				// Title character — use titleGradient
				gIdx := -(cell.colorI + 1)
				style := lipgloss.NewStyle().Foreground(titleGradient[gIdx]).Bold(true)
				sb.WriteString(style.Render(string(cell.ch)))
			} else if cell.settled {
				// Settled torus particle — continuous depth gradient
				gi := int(cell.brightness * float64(len(torusDepthGradient)-1))
				if gi < 0 {
					gi = 0
				} else if gi >= len(torusDepthGradient) {
					gi = len(torusDepthGradient) - 1
				}
				style := lipgloss.NewStyle().Foreground(torusDepthGradient[gi])
				sb.WriteString(style.Render(string(cell.ch)))
			} else {
				// Flowing particle — colorful palette
				idx := cell.colorI % len(flowParticleStyles)
				sb.WriteString(flowParticleStyles[idx].Render(string(cell.ch)))
			}
		}
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ── Public entry point ────────────────────────────────────────────────────────

// RunStartup shows an interactive provider/model selection menu.
// Returns a SetupResult with provider, model, and optional config overrides.
// If skipStartup is true, returns an empty SetupResult.
func RunStartup(skipStartup bool, agentCfg config.AgentConfig) SetupResult {
	if skipStartup {
		return SetupResult{}
	}

	m := newSetupModel()
	m.savedConfig = &agentCfg

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
