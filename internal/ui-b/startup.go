package uib

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
func fetchOpenRouterModels() []startupModelCategory {
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

	toChoice := func(m openRouterModel) startupModelChoice {
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
		return startupModelChoice{
			Name:          m.Name + tag,
			ID:            m.ID,
			ContextWindow: m.ContextLength,
			MaxTokens:     maxTokens,
		}
	}

	// Build FREE MODELS category
	var freeChoices []startupModelChoice
	for _, m := range textModels {
		if m.Pricing.Prompt == "0" && m.Pricing.Completion == "0" {
			freeChoices = append(freeChoices, toChoice(m))
		}
	}
	freeChoices = append(freeChoices, startupModelChoice{Name: "Custom model ID", ID: ""})

	// Build per-author categories
	authorOrder := []string{}
	authorModels := map[string][]startupModelChoice{}
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
	categories := []startupModelCategory{
		{Name: "FREE MODELS", Models: freeChoices},
	}
	for _, author := range authorOrder {
		models := authorModels[author]
		models = append(models, startupModelChoice{Name: "Custom model ID", ID: ""})
		categories = append(categories, startupModelCategory{Name: author, Models: models})
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
	"qwen/qwen3.5-122b-a10b":                        true,
	"z-ai/glm4.7":                                    true,
	"z-ai/glm5":                                      true,
	"stepfun-ai/step-3.5-flash":                      true,
	"minimaxai/minimax-m2.1":                         true,
	"minimaxai/minimax-m2.5":                         true,
	"deepseek-ai/deepseek-v3.2":                      true,
	"deepseek-ai/deepseek-v3.1":                      true,
	"deepseek-ai/deepseek-v3.1-terminus":             true,
	"mistralai/devstral-2-123b-instruct-2512":        true,
	"moonshotai/kimi-k2-thinking":                    true,
	"moonshotai/kimi-k2-instruct":                    true,
	"mistralai/mistral-large-3-675b-instruct-2512":   true,
	"mistralai/magistral-small-2506":                 true,
	"mistralai/mamba-codestral-7b-v01":               true,
	"mistralai/mistral-nemo-minitron-8b-8k-instruct": true,
	"bytedance/seed-oss-36b-instruct":                true,
	"qwen/qwen3-coder-480b-a35b-instruct":            true,
	"openai/gpt-oss-20b":                             true,
	"openai/gpt-oss-120b":                            true,
	"google/gemma-3-27b-it":                          true,
	"google/gemma-2-2b-it":                           true,
	"google/gemma-3n-e4b-it":                         true,
	"google/shieldgemma-9b":                          true,
	"igenius/colosseum_355b_instruct_16k":             true,
	"tiiuae/falcon3-7b-instruct":                     true,
	"igenius/italia_10b_instruct_16k":                 true,
	"nvidia/cosmos-nemotron-34b":                     true,
	"nvidia/cosmos-reason2-8b":                       true,
	"qwen/qwen2.5-coder-7b-instruct":                true,
	"qwen/qwen2-7b-instruct":                        true,
	"abacusai/dracarys-llama-3.1-70b-instruct":      true,
	"thudm/chatglm3-6b":                             true,
	"baichuan-inc/baichuan2-13b-chat":                true,
	"nvidia/nemotron-3-super-120b-a12b":              true,
	"nvidia/nemotron-3-nano-30b-a3b":                 true,
	"nvidia/nvidia-nemotron-nano-9b-v2":              true,
	"nvidia/llama-3.3-nemotron-super-49b-v1":         true,
	"nvidia/llama-3.3-nemotron-super-49b-v1.5":       true,
	"nvidia/nemotron-content-safety-reasoning-4b":    true,
	"nvidia/llama-3.1-nemotron-safety-guard-8b-v3":   true,
	"nvidia/llama-3.1-nemotron-70b-reward":           true,
	"marin/marin-8b-instruct":                       true,
	"nv-mistralai/mistral-nemo-12b-instruct":         true,
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
func fetchNvidiaNIMModels() []startupModelCategory {
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

	toChoice := func(m nvidiaNIMModel) startupModelChoice {
		tag := ""
		if nvidiaNIMFreeModels[m.ID] {
			tag = " [Free]"
		}
		return startupModelChoice{
			Name:          m.ID + tag,
			ID:            m.ID,
			ContextWindow: 0,
			MaxTokens:     0,
		}
	}

	// Build FREE MODELS category
	freeChoices := []startupModelChoice{
		{Name: "Free Models Router [Free]", ID: "nvidia/free", ContextWindow: 131072, MaxTokens: 8192},
	}
	for _, m := range chatModels {
		if nvidiaNIMFreeModels[m.ID] {
			freeChoices = append(freeChoices, toChoice(m))
		}
	}
	freeChoices = append(freeChoices, startupModelChoice{Name: "Custom model ID", ID: ""})

	// Build per-owner categories
	ownerOrder := []string{}
	ownerModels := map[string][]startupModelChoice{}
	for _, m := range chatModels {
		owner := m.OwnedBy
		if owner == "" {
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
	categories := []startupModelCategory{
		{Name: "FREE MODELS", Models: freeChoices},
	}
	for _, owner := range ownerOrder {
		models := ownerModels[owner]
		models = append(models, startupModelChoice{Name: "Custom model ID", ID: ""})
		categories = append(categories, startupModelCategory{Name: owner, Models: models})
	}

	return categories
}

// ── Setup result and config overrides ────────────────────────────────────────

// StartupResult holds the configuration choices from the startup screen.
type StartupResult struct {
	Provider string
	Model    string
	Config   *StartupConfigOverrides // nil means "use existing config.json"
}

// StartupConfigOverrides holds agent settings that can be set during setup.
type StartupConfigOverrides struct {
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
	RewardScoring          bool
	ForceStream            bool
}

func defaultStartupOverrides() *StartupConfigOverrides {
	return overridesFromAgentConfig(config.DefaultAgentConfig())
}

// overridesFromAgentConfig creates config overrides from an AgentConfig.
// MaxTokens and ContextWindow are set to 0 ("default") to allow auto-resolution.
func overridesFromAgentConfig(a config.AgentConfig) *StartupConfigOverrides {
	mode := a.SteeringMode
	if mode == "" {
		mode = "mild"
	}
	return &StartupConfigOverrides{
		MaxTokens:              0,
		ContextWindow:          0,
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
		ForceStream:            a.ForceStream,
	}
}

// ── Config fields ────────────────────────────────────────────────────────────

type startupConfigField struct {
	name    string
	kind    string   // "bool", "int", "string", "cycle"
	options []string // for "cycle" kind only
}

var startupConfigFields = []startupConfigField{
	{"Compaction", "cycle", []string{"llm", "sliding", "off"}},             // 0
	{"CompactionTrigger", "cycle", []string{"both", "tokens", "messages"}}, // 1
	{"CompactionThreshold", "int", nil},                                    // 2
	{"CompactionMaxMessages", "int", nil},                                  // 3
	{"CompactionKeepLastN", "int", nil},                                    // 4
	{"CompactionModel", "string", nil},                                     // 5
	{"ContinuousCompression", "bool", nil},                                 // 6
	{"CompressionKeepFirst", "int", nil},                                   // 7
	{"CompressionKeepLast", "int", nil},                                    // 8
	{"CompressionMinMessages", "int", nil},                                 // 9
	{"ZoneBudgeting", "bool", nil},                                         // 10
	{"ZoneArchivePercent", "int", nil},                                     // 11
	{"SmartRouting", "bool", nil},                                          // 12
	{"SmartRoutingModel", "string", nil},                                   // 13
	{"SteeringMode", "cycle", []string{"mild", "aggressive"}},              // 14
	{"PersistThinking", "bool", nil},                                       // 15
	{"Thinking", "cycle", []string{"", "low", "mid", "high", "max", "ultra"}}, // 16
	{"ThinkingBudget", "int", nil},                                         // 17
	{"MaxTokens", "int", nil},                                              // 18
	{"ContextWindow", "int", nil},                                          // 19
	{"ForceStream", "bool", nil},                                           // 20
}

// formatStartupProviderModel formats "provider:model" as "model (provider)" for display.
func formatStartupProviderModel(pm string) string {
	if idx := strings.IndexByte(pm, ':'); idx > 0 {
		return pm[idx+1:] + " (" + pm[:idx] + ")"
	}
	return pm
}

func (o *StartupConfigOverrides) getValue(idx int) string {
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
		return formatStartupProviderModel(o.CompactionModel)
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
		return formatStartupProviderModel(o.SmartRoutingModel)
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
	case 20:
		if o.ForceStream {
			return "true"
		}
		return "false"
	}
	return ""
}

func (o *StartupConfigOverrides) setValue(idx int, val string) {
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
	case 20:
		o.ForceStream = val == "true"
	}
}

func (o *StartupConfigOverrides) toggleBool(idx int) {
	switch idx {
	case 6:
		o.ContinuousCompression = !o.ContinuousCompression
	case 10:
		o.ZoneBudgeting = !o.ZoneBudgeting
	case 12:
		o.SmartRouting = !o.SmartRouting
	case 15:
		o.PersistThinking = !o.PersistThinking
	case 20:
		o.ForceStream = !o.ForceStream
	}
}

// cycleOption cycles through the options for a cycle field.
// dir: +1 = right/forward, -1 = left/backward
func (o *StartupConfigOverrides) cycleOption(idx, dir int) {
	f := startupConfigFields[idx]
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

// ── Provider/Model types ─────────────────────────────────────────────────────

// startupAuthMethod represents an authentication option for a provider.
type startupAuthMethod struct {
	Name     string // display name, e.g. "OAuth", "API key"
	NeedsKey string // env var name ("" for OAuth)
}

// startupModelCategory groups models under a named category.
type startupModelCategory struct {
	Name   string
	Models []startupModelChoice
}

// startupProviderGroup groups models and auth methods under a single provider.
type startupProviderGroup struct {
	Name        string                 // display name
	ProviderKey string                 // "anthropic", "openrouter", etc. ("" = custom)
	AuthMethods []startupAuthMethod    // if len > 1, user picks auth before model
	Models      []startupModelChoice   // direct models (used when Categories is empty)
	Categories  []startupModelCategory // if set, user picks category before model
}

// startupModelChoice is a single model within a provider group.
type startupModelChoice struct {
	Name          string
	ID            string
	ContextWindow int
	MaxTokens     int
}

// startupModelPickerEntry is a flattened model entry with provider context.
type startupModelPickerEntry struct {
	Label       string
	ModelID     string
	ProviderKey string
}

// ProviderModelID returns "provider:model" format for storage in config.
func (e startupModelPickerEntry) ProviderModelID() string {
	if e.ModelID == "" {
		return ""
	}
	return e.ProviderKey + ":" + e.ModelID
}

// buildStartupModelPickerItems gathers all models from all provider groups into a flat list.
func buildStartupModelPickerItems(groups []startupProviderGroup) []startupModelPickerEntry {
	var items []startupModelPickerEntry
	items = append(items, startupModelPickerEntry{Label: "(none -- use main model)", ModelID: "", ProviderKey: ""})
	for _, g := range groups {
		provName := g.Name
		if g.ProviderKey == "" {
			continue
		}
		for _, mc := range g.Models {
			if mc.ID == "" {
				continue
			}
			items = append(items, startupModelPickerEntry{
				Label:       mc.Name + " (" + provName + ")",
				ModelID:     mc.ID,
				ProviderKey: g.ProviderKey,
			})
		}
		for _, cat := range g.Categories {
			for _, mc := range cat.Models {
				if mc.ID == "" {
					continue
				}
				items = append(items, startupModelPickerEntry{
					Label:       mc.Name + " (" + provName + " / " + cat.Name + ")",
					ModelID:     mc.ID,
					ProviderKey: g.ProviderKey,
				})
			}
		}
	}
	return items
}

// defaultStartupProviderGroups returns the grouped provider options.
func defaultStartupProviderGroups() []startupProviderGroup {
	return []startupProviderGroup{
		{
			Name: "OpenRouter", ProviderKey: "openrouter",
			AuthMethods: []startupAuthMethod{{Name: "API key", NeedsKey: "OPENROUTER_API_KEY"}},
			Categories: []startupModelCategory{
				{Name: "FREE MODELS", Models: []startupModelChoice{
					{Name: "nemotron-3-super (free)", ID: "nvidia/nemotron-3-super-120b-a12b:free", ContextWindow: 131072, MaxTokens: 8192},
					{Name: "step-3.5-flash (free)", ID: "stepfun/step-3.5-flash:free", ContextWindow: 128000, MaxTokens: 8192},
					{Name: "Custom model ID", ID: ""},
				}},
			},
		},
		{
			Name: "NVIDIA NIM", ProviderKey: "nvidia",
			AuthMethods: []startupAuthMethod{{Name: "API key", NeedsKey: "NVIDIA_API_KEY"}},
			Categories: []startupModelCategory{
				{Name: "FREE MODELS", Models: []startupModelChoice{
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
			AuthMethods: []startupAuthMethod{
				{Name: "OAuth (no key needed)", NeedsKey: ""},
				{Name: "API key", NeedsKey: "ANTHROPIC_API_KEY"},
			},
			Models: []startupModelChoice{
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
			AuthMethods: []startupAuthMethod{{Name: "API key", NeedsKey: "OPENAI_API_KEY"}},
			Models: []startupModelChoice{
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
			AuthMethods: []startupAuthMethod{{Name: "API key", NeedsKey: "XAI_API_KEY"}},
			Models: []startupModelChoice{
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
			AuthMethods: []startupAuthMethod{{Name: "API key", NeedsKey: "GEMINI_API_KEY"}},
			Models: []startupModelChoice{
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
			AuthMethods: []startupAuthMethod{{Name: "API key", NeedsKey: "AZURE_OPENAI_API_KEY"}},
			Models: []startupModelChoice{
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
			AuthMethods: []startupAuthMethod{{Name: "Access token", NeedsKey: "VERTEX_ACCESS_TOKEN"}},
			Models: []startupModelChoice{
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

// ── Styles (amber/orange, matching the original) ─────────────────────────────

var (
	suBrightAmber = lipgloss.Color("166")
	suOrange      = lipgloss.Color("130")
	suMutedGold   = lipgloss.Color("130")
)

var (
	suTitleStyle = lipgloss.NewStyle().
			Foreground(suBrightAmber).
			Bold(true)

	suSubtitleStyle = lipgloss.NewStyle().
			Foreground(suMutedGold).
			Italic(true)

	suMenuPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(suOrange).
				Padding(1, 2)

	suMenuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	suMenuSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(suBrightAmber).
				Bold(true).
				Padding(0, 1)

	suMenuHeaderStyle = lipgloss.NewStyle().
				Foreground(suOrange).
				Bold(true).
				Underline(true).
				MarginBottom(1)

	suFooterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242")).
			Italic(true)

	// Torus character luminance styles (neon orange gradient).
	suTorusDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#993300"))
	suTorusMidStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#cc4400"))
	suTorusBrightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4d01"))
	suTorusMaxStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6600"))

	suTextInputStyle = lipgloss.NewStyle().
				Foreground(suBrightAmber).
				Bold(true)

	suPromptLabelStyle = lipgloss.NewStyle().
				Foreground(suMutedGold)

	suScrollIndicatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242")).
				Italic(true)
)

// Maximum number of menu items visible before scrolling kicks in.
const startupVisibleItems = 10

// ── ASCII art title ──────────────────────────────────────────────────────────

const startupASCIITitle = "" +
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
var startupTitleGradient = []lipgloss.Color{
	lipgloss.Color("#662200"),
	lipgloss.Color("#993300"),
	lipgloss.Color("#cc4400"),
	lipgloss.Color("#ff4d01"),
	lipgloss.Color("#ff6600"),
	lipgloss.Color("#ff8800"),
	lipgloss.Color("#ffaa00"),
	lipgloss.Color("#ff8800"),
	lipgloss.Color("#ff6600"),
	lipgloss.Color("#ff4d01"),
	lipgloss.Color("#cc4400"),
	lipgloss.Color("#993300"),
}

// renderStartupAnimatedTitle colors each column of the ASCII title with a wave
// of neon orange that shifts over time based on the torus rotation angle.
func renderStartupAnimatedTitle(title string, phase float64) string {
	lines := strings.Split(title, "\n")
	gradLen := len(startupTitleGradient)
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
			idx := (col + int(phase*3)) % gradLen
			if idx < 0 {
				idx += gradLen
			}
			style := lipgloss.NewStyle().Foreground(startupTitleGradient[idx]).Bold(true)
			sb.WriteString(style.Render(string(ch)))
		}
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ── Tick command for startup animation ───────────────────────────────────────

// startupTickMsg drives the startup torus animation at 50ms intervals.
// Separate from TickMsg to avoid collision with the progress bar tick.
type startupTickMsg time.Time

func startupTickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return startupTickMsg(t)
	})
}

// ── Startup model ────────────────────────────────────────────────────────────

type startupModel struct {
	width, height int
	ready         bool

	// Torus animation
	torusA, torusB float64
	torusFrame     string

	// Menu phases:
	// 0=main, 1=pick provider, 2=pick auth, 3=pick category (if any), 4=pick model,
	// 5=config mode, 6=edit settings, 7=model picker (for compactionModel/smartRoutingModel)
	phase        int
	cursor       int
	scrollOffset int

	// Provider groups (phase 1)
	groups []startupProviderGroup

	// Current selection state
	selectedGroup    *startupProviderGroup
	selectedAuth     *startupAuthMethod
	selectedCategory *startupModelCategory

	// Text input for custom provider/model
	textInput string
	inputMode bool
	inputStep int // 0=provider name, 1=model id

	// Type-to-filter for list screens (phases 1, 3, 4)
	filterText string

	// Config customization (phase 5 = choose config mode, phase 6 = edit settings)
	savedConfig     *config.AgentConfig
	configOverrides *StartupConfigOverrides
	editingConfig   bool
	editBuffer      string

	// Model picker (phase 7)
	modelPickerItems []startupModelPickerEntry
	modelPickerField int

	// Result
	provider string
	model    string
	done     bool
	quit     bool // ctrl+c or q to quit entirely
}

func newStartupModel() startupModel {
	groups := defaultStartupProviderGroups()

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

	m := startupModel{groups: groups}
	m.torusFrame = renderStartupTorus(m.torusA, m.torusB)
	return m
}

func (m startupModel) savedOverrides() *StartupConfigOverrides {
	if m.savedConfig != nil {
		return overridesFromAgentConfig(*m.savedConfig)
	}
	return defaultStartupOverrides()
}

// ── Update ───────────────────────────────────────────────────────────────────

func (m startupModel) Update(msg tea.Msg) (startupModel, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil

	case startupTickMsg:
		m.torusA += 0.04
		m.torusB += 0.02
		m.torusFrame = renderStartupTorus(m.torusA, m.torusB)
		return m, startupTickCmd()

	case tea.KeyMsg:
		return m.handleStartupKey(msg)
	}

	return m, nil
}

func (m startupModel) handleStartupKey(msg tea.KeyMsg) (startupModel, tea.Cmd) {
	// Config editing mode (phase 6 inline edit)
	if m.editingConfig {
		return m.handleStartupConfigEdit(msg)
	}

	// Text input mode
	if m.inputMode {
		return m.handleStartupTextInput(msg)
	}

	// Normal menu navigation
	switch msg.String() {

	case "ctrl+c":
		m.provider = ""
		m.model = ""
		m.done = true
		m.quit = true
		return m, tea.Quit

	case "q":
		if m.startupFilterablePhase() {
			m.filterText += "q"
			m.cursor = 0
			m.scrollOffset = 0
		} else {
			m.provider = ""
			m.model = ""
			m.done = true
			m.quit = true
			return m, tea.Quit
		}

	case "up":
		total := m.startupMenuLen()
		if m.cursor > 0 {
			m.cursor--
		} else {
			m.cursor = total - 1
		}
		m.scrollOffset = startupClampScrollOffset(m.cursor, m.scrollOffset, total)

	case "k":
		if !m.startupFilterablePhase() {
			total := m.startupMenuLen()
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = total - 1
			}
			m.scrollOffset = startupClampScrollOffset(m.cursor, m.scrollOffset, total)
		} else {
			m.filterText += "k"
			m.cursor = 0
			m.scrollOffset = 0
		}

	case "down":
		total := m.startupMenuLen()
		if m.cursor < total-1 {
			m.cursor++
		} else {
			m.cursor = 0
		}
		m.scrollOffset = startupClampScrollOffset(m.cursor, m.scrollOffset, total)

	case "j":
		if !m.startupFilterablePhase() {
			total := m.startupMenuLen()
			if m.cursor < total-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
			m.scrollOffset = startupClampScrollOffset(m.cursor, m.scrollOffset, total)
		} else {
			m.filterText += "j"
			m.cursor = 0
			m.scrollOffset = 0
		}

	case "enter", " ":
		return m.startupSelectItem()

	case "left":
		if m.phase == 6 && m.cursor < len(startupConfigFields) {
			f := startupConfigFields[m.cursor]
			if f.kind == "cycle" {
				m.configOverrides.cycleOption(m.cursor, -1)
			} else if f.kind == "bool" {
				m.configOverrides.toggleBool(m.cursor)
			}
		}

	case "h":
		if !m.startupFilterablePhase() {
			if m.phase == 6 && m.cursor < len(startupConfigFields) {
				f := startupConfigFields[m.cursor]
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
		if m.phase == 6 && m.cursor < len(startupConfigFields) {
			f := startupConfigFields[m.cursor]
			if f.kind == "cycle" {
				m.configOverrides.cycleOption(m.cursor, 1)
			} else if f.kind == "bool" {
				m.configOverrides.toggleBool(m.cursor)
			}
		}

	case "l":
		if !m.startupFilterablePhase() {
			if m.phase == 6 && m.cursor < len(startupConfigFields) {
				f := startupConfigFields[m.cursor]
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
		case 7:
			m.phase = 6
			m.cursor = m.modelPickerField
			m.scrollOffset = 0
			m.modelPickerItems = nil
		case 6:
			m.phase = 5
			m.cursor = 0
			m.scrollOffset = 0
		case 5:
			m.phase = 4
			m.cursor = 0
			m.scrollOffset = 0
		case 4:
			if m.selectedCategory != nil {
				m.phase = 3
			} else if m.selectedGroup != nil && len(m.selectedGroup.AuthMethods) > 1 {
				m.phase = 2
			} else {
				m.phase = 1
			}
			m.cursor = 0
			m.scrollOffset = 0
		case 3:
			m.selectedCategory = nil
			if m.selectedGroup != nil && len(m.selectedGroup.AuthMethods) > 1 {
				m.phase = 2
			} else {
				m.phase = 1
			}
			m.cursor = 0
			m.scrollOffset = 0
		case 2:
			m.phase = 1
			m.cursor = 0
			m.scrollOffset = 0
			m.selectedAuth = nil
		case 1:
			m.phase = 0
			m.cursor = 0
			m.scrollOffset = 0
			m.selectedGroup = nil
		}

	default:
		// Type-to-filter on list screens
		if m.startupFilterablePhase() {
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

func (m startupModel) handleStartupConfigEdit(msg tea.KeyMsg) (startupModel, tea.Cmd) {
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

func (m startupModel) handleStartupTextInput(msg tea.KeyMsg) (startupModel, tea.Cmd) {
	switch msg.String() {

	case "ctrl+c":
		m.provider = ""
		m.model = ""
		m.done = true
		m.quit = true
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
			m.model = val
			m.inputMode = false
			m.textInput = ""
			m.inputStep = 0
			m.phase = 5
			m.cursor = 0
			m.scrollOffset = 0
			m.configOverrides = m.savedOverrides()
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
			return m, nil
		}
		return m, nil

	case "backspace":
		if len(m.textInput) > 0 {
			m.textInput = m.textInput[:len(m.textInput)-1]
		}

	default:
		if len(msg.String()) == 1 {
			m.textInput += msg.String()
		}
	}

	return m, nil
}

// ── Filter helpers ───────────────────────────────────────────────────────────

func startupFilteredIndices(total int, labelFn func(i int) string, filter string) []int {
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

func (m startupModel) startupFilterablePhase() bool {
	return m.phase == 1 || m.phase == 3 || m.phase == 4 || m.phase == 7
}

func (m startupModel) startupCurrentModels() []startupModelChoice {
	if m.selectedCategory != nil {
		return m.selectedCategory.Models
	}
	if m.selectedGroup != nil {
		return m.selectedGroup.Models
	}
	return nil
}

func (m startupModel) startupMenuLen() int {
	if m.filterText != "" && m.startupFilterablePhase() {
		return len(m.startupFilteredItems())
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
	case 3:
		if m.selectedGroup != nil {
			return len(m.selectedGroup.Categories)
		}
		return 0
	case 4:
		return len(m.startupCurrentModels())
	case 5:
		return 3
	case 6:
		return len(startupConfigFields) + 1
	case 7:
		return len(m.modelPickerItems)
	}
	return 0
}

func (m startupModel) startupFilteredItems() []int {
	switch m.phase {
	case 1:
		return startupFilteredIndices(len(m.groups), func(i int) string {
			return m.groups[i].Name
		}, m.filterText)
	case 3:
		if m.selectedGroup != nil {
			return startupFilteredIndices(len(m.selectedGroup.Categories), func(i int) string {
				return m.selectedGroup.Categories[i].Name
			}, m.filterText)
		}
	case 4:
		models := m.startupCurrentModels()
		return startupFilteredIndices(len(models), func(i int) string {
			return models[i].Name
		}, m.filterText)
	case 7:
		return startupFilteredIndices(len(m.modelPickerItems), func(i int) string {
			return m.modelPickerItems[i].Label
		}, m.filterText)
	}
	return nil
}

func (m startupModel) startupResolveFilteredIndex() int {
	if m.filterText == "" || !m.startupFilterablePhase() {
		return m.cursor
	}
	items := m.startupFilteredItems()
	if m.cursor < len(items) {
		return items[m.cursor]
	}
	return 0
}

// startupClampScrollOffset returns an adjusted scrollOffset that keeps cursor
// within the visible window.
func startupClampScrollOffset(cursor, scrollOffset, total int) int {
	if total <= startupVisibleItems {
		return 0
	}
	if cursor < scrollOffset {
		return cursor
	}
	if cursor >= scrollOffset+startupVisibleItems {
		return cursor - startupVisibleItems + 1
	}
	return scrollOffset
}

// ── Selection logic ──────────────────────────────────────────────────────────

func (m startupModel) startupNextModelPhase() int {
	if m.selectedGroup != nil && len(m.selectedGroup.Categories) > 0 {
		return 3
	}
	return 4
}

func (m startupModel) startupSelectItem() (startupModel, tea.Cmd) {
	switch m.phase {

	case 0: // Main menu
		switch m.cursor {
		case 0: // Use existing config
			m.provider = ""
			m.model = ""
			m.done = true
			return m, nil
		case 1: // Choose provider & model
			m.phase = 1
			m.cursor = 0
			m.scrollOffset = 0
		}

	case 1: // Pick provider group
		idx := m.startupResolveFilteredIndex()
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
			m.phase = m.startupNextModelPhase()
			m.cursor = 0
			m.scrollOffset = 0
		}
		return m, nil

	case 2: // Pick auth method
		auth := m.selectedGroup.AuthMethods[m.cursor]
		m.selectedAuth = &auth
		m.phase = m.startupNextModelPhase()
		m.cursor = 0
		m.scrollOffset = 0
		return m, nil

	case 3: // Pick category
		idx := m.startupResolveFilteredIndex()
		m.filterText = ""
		cat := m.selectedGroup.Categories[idx]
		m.selectedCategory = &cat
		m.phase = 4
		m.cursor = 0
		m.scrollOffset = 0
		return m, nil

	case 4: // Pick model
		idx := m.startupResolveFilteredIndex()
		m.filterText = ""
		models := m.startupCurrentModels()
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
		}
		return m, nil

	case 5: // Config mode
		switch m.cursor {
		case 0:
			m.configOverrides = m.savedOverrides()
			m.done = true
			return m, nil
		case 1:
			m.configOverrides = nil
			m.done = true
			return m, nil
		case 2:
			if m.configOverrides == nil {
				m.configOverrides = m.savedOverrides()
			}
			m.phase = 6
			m.cursor = 0
			m.scrollOffset = 0
		}

	case 6: // Edit config
		if m.cursor >= len(startupConfigFields) {
			m.done = true
			return m, nil
		}
		f := startupConfigFields[m.cursor]
		// CompactionModel (5) and SmartRoutingModel (12) open model picker
		if m.cursor == 5 || m.cursor == 12 {
			m.modelPickerItems = buildStartupModelPickerItems(m.groups)
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
		idx := m.startupResolveFilteredIndex()
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

// ── View ─────────────────────────────────────────────────────────────────────

func (m startupModel) View() string {
	if !m.ready {
		return ""
	}

	var b strings.Builder

	// Title block (animated neon gradient)
	titleRendered := renderStartupAnimatedTitle(startupASCIITitle, m.torusA)
	titleBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, titleRendered)
	b.WriteString(titleBlock)
	b.WriteString("\n\n")

	// Menu panel
	menuContent := m.renderStartupMenu()
	menuPanel := suMenuPanelStyle.Render(menuContent)

	// Layout: torus left, menu right (or menu only if narrow)
	narrow := m.width < 80
	if narrow {
		centered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, menuPanel)
		b.WriteString(centered)
	} else {
		coloredTorus := colorStartupTorus(m.torusFrame)
		torusPanel := lipgloss.NewStyle().
			Width(38).
			Render(coloredTorus)

		joined := lipgloss.JoinHorizontal(lipgloss.Top, torusPanel, "  ", menuPanel)
		centered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, joined)
		b.WriteString(centered)
	}

	b.WriteString("\n\n")

	// Footer hints
	var hint string
	if m.inputMode {
		hint = "enter: confirm  |  esc: cancel  |  ctrl+c: quit"
	} else {
		phaseHints := []string{
			"j/k or arrows: navigate  |  enter: select  |  q: quit",
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",
			"j/k or arrows: navigate  |  enter: select  |  esc: back  |  q: quit",
			"j/k: navigate  |  enter: toggle/edit  |  esc: back  |  q: quit",
		}
		if m.phase < len(phaseHints) {
			hint = phaseHints[m.phase]
		}
	}
	footer := suFooterStyle.Render(hint)
	footerBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer)
	b.WriteString(footerBlock)

	return b.String()
}

// renderScrollableStartupItems writes a windowed slice of items into the builder.
func (m startupModel) renderScrollableStartupItems(b *strings.Builder, total int, labelFn func(i int) string) {
	if total == 0 {
		return
	}

	start := m.scrollOffset
	end := start + startupVisibleItems
	if end > total || total <= startupVisibleItems {
		end = total
		start = end - startupVisibleItems
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		b.WriteString(suScrollIndicatorStyle.Render(fmt.Sprintf("  ... %d more above", start)))
		b.WriteByte('\n')
	}

	for i := start; i < end; i++ {
		label := labelFn(i)
		if i == m.cursor {
			b.WriteString(suMenuSelectedStyle.Render("> " + label))
		} else {
			b.WriteString(suMenuItemStyle.Render("  " + label))
		}
		b.WriteByte('\n')
	}

	if end < total {
		b.WriteString(suScrollIndicatorStyle.Render(fmt.Sprintf("  ... %d more below", total-end)))
		b.WriteByte('\n')
	}
}

// renderScrollableStartupSettings renders phase-6 config fields with scroll support.
func (m startupModel) renderScrollableStartupSettings(b *strings.Builder, total int) {
	if total == 0 {
		return
	}

	start := m.scrollOffset
	end := start + startupVisibleItems
	if end > total || total <= startupVisibleItems {
		end = total
		start = end - startupVisibleItems
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		b.WriteString(suScrollIndicatorStyle.Render(fmt.Sprintf("  ... %d more above", start)))
		b.WriteByte('\n')
	}

	doneIdx := len(startupConfigFields)
	for i := start; i < end; i++ {
		if i == doneIdx {
			if m.cursor == doneIdx {
				b.WriteString(suMenuSelectedStyle.Render("> Done"))
			} else {
				b.WriteString(suMenuItemStyle.Render("  Done"))
			}
			b.WriteByte('\n')
			continue
		}
		f := startupConfigFields[i]
		val := m.configOverrides.getValue(i)
		var line string
		if f.kind == "bool" {
			indicator := "\u25cb"
			if val == "true" {
				indicator = "\u25cf"
			}
			line = fmt.Sprintf("%s %s", indicator, f.name)
		} else {
			line = fmt.Sprintf("%s: %s", f.name, val)
		}
		if i == m.cursor {
			if m.editingConfig {
				line = fmt.Sprintf("%s: %s_", f.name, m.editBuffer)
				b.WriteString(suTextInputStyle.Render("> " + line))
			} else {
				b.WriteString(suMenuSelectedStyle.Render("> " + line))
			}
		} else {
			b.WriteString(suMenuItemStyle.Render("  " + line))
		}
		b.WriteByte('\n')
	}

	if end < total {
		b.WriteString(suScrollIndicatorStyle.Render(fmt.Sprintf("  ... %d more below", total-end)))
		b.WriteByte('\n')
	}
}

func (m startupModel) renderStartupMenu() string {
	var b strings.Builder

	switch m.phase {
	case 0:
		b.WriteString(suMenuHeaderStyle.Render("Setup"))
		b.WriteByte('\n')
		items := []string{
			"Use existing config",
			"Choose provider & model",
		}
		m.renderScrollableStartupItems(&b, len(items), func(i int) string {
			return items[i]
		})

	case 1:
		b.WriteString(suMenuHeaderStyle.Render("Select Provider"))
		b.WriteByte('\n')
		if m.filterText != "" {
			b.WriteString(suTextInputStyle.Render("  / " + m.filterText + "_"))
			b.WriteByte('\n')
		}
		indices := m.startupFilteredItems()
		m.renderScrollableStartupItems(&b, len(indices), func(i int) string {
			return m.groups[indices[i]].Name
		})

	case 2:
		header := "Authentication"
		if m.selectedGroup != nil {
			header = m.selectedGroup.Name + " -- Authentication"
		}
		b.WriteString(suMenuHeaderStyle.Render(header))
		b.WriteByte('\n')
		if m.selectedGroup != nil {
			auths := m.selectedGroup.AuthMethods
			m.renderScrollableStartupItems(&b, len(auths), func(i int) string {
				return auths[i].Name
			})
		}

	case 3:
		header := "Select Category"
		if m.selectedGroup != nil {
			header = m.selectedGroup.Name + " -- Select Category"
		}
		b.WriteString(suMenuHeaderStyle.Render(header))
		b.WriteByte('\n')
		if m.filterText != "" {
			b.WriteString(suTextInputStyle.Render("  / " + m.filterText + "_"))
			b.WriteByte('\n')
		}
		if m.selectedGroup != nil {
			cats := m.selectedGroup.Categories
			indices := m.startupFilteredItems()
			m.renderScrollableStartupItems(&b, len(indices), func(i int) string {
				return fmt.Sprintf("%s (%d)", cats[indices[i]].Name, len(cats[indices[i]].Models))
			})
		}

	case 4:
		header := "Select Model"
		if m.selectedGroup != nil {
			if m.selectedCategory != nil {
				header = m.selectedGroup.Name + " -- " + m.selectedCategory.Name
			} else {
				header = m.selectedGroup.Name + " -- Select Model"
			}
		}
		b.WriteString(suMenuHeaderStyle.Render(header))
		b.WriteByte('\n')
		if m.filterText != "" {
			b.WriteString(suTextInputStyle.Render("  / " + m.filterText + "_"))
			b.WriteByte('\n')
		}
		models := m.startupCurrentModels()
		indices := m.startupFilteredItems()
		m.renderScrollableStartupItems(&b, len(indices), func(i int) string {
			return models[indices[i]].Name
		})

	case 5:
		b.WriteString(suMenuHeaderStyle.Render("Configuration"))
		b.WriteByte('\n')
		items := []string{
			"Use defaults",
			"Use existing config",
			"Customize settings",
		}
		m.renderScrollableStartupItems(&b, len(items), func(i int) string {
			return items[i]
		})

	case 6:
		b.WriteString(suMenuHeaderStyle.Render("Settings"))
		b.WriteByte('\n')
		total := len(startupConfigFields) + 1
		m.renderScrollableStartupSettings(&b, total)

	case 7:
		fieldName := startupConfigFields[m.modelPickerField].name
		b.WriteString(suMenuHeaderStyle.Render("Select " + fieldName))
		b.WriteByte('\n')
		if m.filterText != "" {
			b.WriteString(suTextInputStyle.Render("  / " + m.filterText + "_"))
			b.WriteByte('\n')
		}
		indices := m.startupFilteredItems()
		m.renderScrollableStartupItems(&b, len(indices), func(i int) string {
			return m.modelPickerItems[indices[i]].Label
		})
	}

	// Text input overlay
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
		b.WriteString(suPromptLabelStyle.Render(label))
		b.WriteString(suTextInputStyle.Render(m.textInput))
		b.WriteString(lipgloss.NewStyle().
			Foreground(suBrightAmber).
			Blink(true).
			Render("_"))
	}

	return b.String()
}

// ── 3D Torus rendering (horn torus from horntorus.com) ───────────────────────

func renderStartupTorus(a, b float64) string {
	const (
		width  = 36
		height = 18

		r1      = 1.0  // tube (minor) radius
		r2      = 2.0  // torus (major) radius
		k2      = 5.0  // distance from viewer
		thetaSp = 0.03 // theta step
		phiSp   = 0.01 // phi step
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

// colorStartupTorus applies amber-shade lipgloss styles to each character
// based on its luminance bucket in the donut.c character ramp.
func colorStartupTorus(frame string) string {
	var sb strings.Builder
	for _, ch := range frame {
		switch {
		case ch == '\n':
			sb.WriteByte('\n')
		case ch == ' ':
			sb.WriteByte(' ')
		case ch == '.' || ch == ',' || ch == '-':
			sb.WriteString(suTorusDimStyle.Render(string(ch)))
		case ch == '~' || ch == ':' || ch == ';':
			sb.WriteString(suTorusMidStyle.Render(string(ch)))
		case ch == '=' || ch == '!' || ch == '*':
			sb.WriteString(suTorusBrightStyle.Render(string(ch)))
		case ch == '#' || ch == '$' || ch == '@':
			sb.WriteString(suTorusMaxStyle.Render(string(ch)))
		default:
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// ── Public entry point ───────────────────────────────────────────────────────

// StartupDoneMsg is sent when the startup screen finishes and the main chat
// should begin. It carries the provider, model, and config overrides selected.
type StartupDoneMsg struct {
	Result StartupResult
}

// startupWrapper wraps startupModel to satisfy tea.Model for standalone use.
type startupWrapper struct {
	inner startupModel
}

func (w startupWrapper) Init() tea.Cmd {
	return startupTickCmd()
}

func (w startupWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := w.inner.Update(msg)
	w.inner = updated
	return w, cmd
}

func (w startupWrapper) View() string {
	return w.inner.View()
}

// RunStartupScreen shows the startup screen as a standalone program.
// Returns a StartupResult with provider, model, and optional config overrides.
// If skipStartup is true, returns an empty StartupResult.
func RunStartupScreen(skipStartup bool, agentCfg config.AgentConfig) StartupResult {
	if skipStartup {
		return StartupResult{}
	}

	m := newStartupModel()
	m.savedConfig = &agentCfg

	p := tea.NewProgram(startupWrapper{inner: m}, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return StartupResult{}
	}

	wrapper := finalModel.(startupWrapper)
	result := wrapper.inner
	if !result.done {
		return StartupResult{}
	}

	return StartupResult{
		Provider: result.provider,
		Model:    result.model,
		Config:   result.configOverrides,
	}
}
