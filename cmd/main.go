package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"torus_go_agent/internal/channels"
	_ "torus_go_agent/internal/channels/telegram" // register telegram channel
	tuichan "torus_go_agent/internal/channels/tui"
	"torus_go_agent/internal/config"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
	"torus_go_agent/internal/providers"
	"torus_go_agent/internal/safety"
	"torus_go_agent/internal/tools"
	"torus_go_agent/internal/ui"
)

// resolveConfigDir returns the config directory, checking in order:
// 1. $TORUS_CONFIG_DIR (explicit override)
// 2. ./config (local dev — if it exists)
// 3. $XDG_CONFIG_HOME/torus_go_agent
// 4. ~/.config/torus_go_agent (created if needed)
func resolveConfigDir() string {
	if env := os.Getenv("TORUS_CONFIG_DIR"); env != "" {
		return env
	}
	if info, err := os.Stat(filepath.Join(".", "config")); err == nil && info.IsDir() {
		return filepath.Join(".", "config")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		d := filepath.Join(xdg, "torus_go_agent")
		os.MkdirAll(d, 0755)
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "config")
	}
	d := filepath.Join(home, ".config", "torus_go_agent")
	os.MkdirAll(d, 0755)
	return d
}

func main() {
	_ = godotenv.Load() // load .env if present
	cfgDir := resolveConfigDir()
	cfg, err := config.LoadConfig(filepath.Join(cfgDir, "config.json"))
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Interactive startup: let user pick provider/model (skip with --no-setup flag)
	skipSetup := false
	for _, arg := range os.Args[1:] {
		if arg == "--no-setup" || arg == "--telegram" {
			skipSetup = true
		}
	}
	setup := ui.RunStartup(skipSetup)
	if setup.Provider != "" {
		cfg.Agent.Provider = setup.Provider
		cfg.Agent.Model = setup.Model
	}
	if setup.Config != nil {
		cfg.Agent.MaxTokens = setup.Config.MaxTokens
		cfg.Agent.ContextWindow = setup.Config.ContextWindow
		cfg.Agent.Compaction = setup.Config.Compaction
		cfg.Agent.CompactionTrigger = setup.Config.CompactionTrigger
		cfg.Agent.CompactionThreshold = setup.Config.CompactionThreshold
		cfg.Agent.CompactionMaxMessages = setup.Config.CompactionMaxMessages
		cfg.Agent.CompactionKeepLastN = setup.Config.CompactionKeepLastN
		cfg.Agent.ContinuousCompression = setup.Config.ContinuousCompression
		cfg.Agent.CompressionKeepLast = setup.Config.CompressionKeepLast
		cfg.Agent.CompressionMinMessages = setup.Config.CompressionMinMessages
		cfg.Agent.ZoneBudgeting = setup.Config.ZoneBudgeting
		cfg.Agent.ZoneArchivePercent = setup.Config.ZoneArchivePercent
		cfg.Agent.SmartRouting = setup.Config.SmartRouting
		if setup.Config.SteeringAggressive {
			cfg.Agent.SteeringMode = "aggressive"
		}
	}

	// Auto-detect model specs: models.json → OpenRouter API → config.json defaults
	models := config.LoadModels(cfgDir)
	if info := config.ResolveModelInfo(cfg.Agent.Model, cfg.Agent.Provider, models); info.ContextWindow > 0 {
		cfg.Agent.ContextWindow = info.ContextWindow
		if info.MaxTokens > 0 {
			cfg.Agent.MaxTokens = info.MaxTokens
		}
		log.Printf("[main] model %s: context=%d, maxTokens=%d (auto-detected)", cfg.Agent.Model, info.ContextWindow, info.MaxTokens)
	}

	soul := config.LoadTorus(cfgDir)
	soul = strings.ReplaceAll(soul, "{{MODEL}}", cfg.Agent.Provider+"/"+cfg.Agent.Model)
	schema := config.LoadSchema(cfgDir)
	key := cfg.APIKey()
	if key == "" && cfg.Agent.Provider == "anthropic" {
		oauthKey, err := providers.GetAnthropicKey()
		if err != nil {
			fmt.Println("No API key. Starting Anthropic OAuth login...")
			creds, loginErr := providers.LoginAnthropic(
				func(u string) { fmt.Println("\nOpen this URL:\n  " + u + "\n") },
				func() (string, error) {
					fmt.Print("Paste code#state: ")
					var input string
					fmt.Scanln(&input)
					return strings.TrimSpace(input), nil
				},
			)
			if loginErr != nil {
				log.Fatalf("OAuth failed: %v", loginErr)
			}
			_ = providers.SaveCredentials(creds)
			key = creds.Access
			fmt.Println("Login successful.")
		} else {
			key = oauthKey
		}
	}
	if key == "" {
		fmt.Fprintln(os.Stderr, "No API key. Set OPENROUTER_API_KEY or ANTHROPIC_API_KEY.")
		os.Exit(1)
	}

	// Create provider
	agentCfg = &cfg.Agent
	prov := makeProvider(cfg.Agent.Provider, key, cfg.Agent.Model)

	// Wire weighted routing + fallback if configured
	router := providers.NewRouter(prov)
	if len(cfg.Agent.Routing) > 0 {
		var entries []providers.RoutingEntry
		for _, r := range cfg.Agent.Routing {
			rKey := config.APIKeyFor(r.Provider)
			rProv := makeProvider(r.Provider, rKey, r.Model)
			router.AddProvider(rProv)
			entries = append(entries, providers.RoutingEntry{
				Key:    r.Provider + ":" + r.Model,
				Weight: r.Weight,
			})
		}
		router.SetWeights(entries)
		log.Printf("[main] weighted routing enabled: %d providers", len(entries))
	}
	if len(cfg.Agent.FallbackOrder) > 0 {
		router.SetFallbackOrder(cfg.Agent.FallbackOrder)
		log.Printf("[main] fallback chain: %v", cfg.Agent.FallbackOrder)
	}

	// Create DAG
	dataDir := cfg.DataDir(cfgDir)
	os.MkdirAll(dataDir, 0755)
	dag, err := core.NewDAG(filepath.Join(dataDir, "conversations.db"))
	if err != nil {
		log.Fatalf("dag: %v", err)
	}
	defer dag.Close()

	// Create hooks + telemetry + safety scanners
	hooks := core.NewHookRegistry()
	telemetry := features.NewTelemetryCollector()
	telemetry.RegisterHooks(hooks)
	hooks.Register(core.HookBeforeToolCall, "secret-scan", func(ctx context.Context, d *core.HookData) error {
		if d.ToolName == "write" || d.ToolName == "edit" {
			content, _ := d.ToolArgs["content"].(string)
			if content == "" {
				content, _ = d.ToolArgs["new_str"].(string)
			}
			if desc, found := safety.ScanSecrets(content); found {
				d.Block = true
				d.BlockReason = "Secret detected: " + desc
			}
		}
		return nil
	})
	hooks.Register(core.HookBeforeToolCall, "danger-scan", func(ctx context.Context, d *core.HookData) error {
		if d.ToolName == "bash" {
			command, _ := d.ToolArgs["command"].(string)
			if label, bad := safety.CheckSafety(command); bad {
				d.Block = true
				d.BlockReason = "Dangerous command: " + label
			}
		}
		return nil
	})

	// Inject live DAG state per turn (static schema now in TORUS.md).
	hooks.Register(core.HookBeforeContextBuild, "dag-context", func(ctx context.Context, d *core.HookData) error {
		brID, brName, headNode, msgCount := dag.CurrentBranchInfo()
		contextLine := fmt.Sprintf("[DAG state] branch: %s (%s), head: %s, messages: %d", brID, brName, headNode, msgCount)
		state := core.Message{
			Role:    core.RoleUser,
			Content: []core.ContentBlock{{Type: "text", Text: contextLine}},
		}
		d.Messages = append([]core.Message{state}, d.Messages...)
		return nil
	})

	// Inject SCHEMA.md as first DAG node on branch start (survives compaction).
	injectSchema := func() {
		if schema == "" {
			return
		}
		head, _ := dag.GetHead()
		if head != "" {
			// Branch already has messages — don't double-inject
			return
		}
		dag.AddNode("", core.RoleUser, []core.ContentBlock{{Type: "text", Text: schema}}, "", "", 0)
	}
	// On app start: inject into current branch if empty
	injectSchema()
	// On /new: inject into the fresh branch
	hooks.Register(core.HookAfterNewBranch, "schema-inject", func(ctx context.Context, d *core.HookData) error {
		injectSchema()
		return nil
	})
	// On /clear: re-inject after head is wiped
	hooks.Register(core.HookPostClear, "schema-inject", func(ctx context.Context, d *core.HookData) error {
		injectSchema()
		return nil
	})

	// Register continuous compression hook (optional, config-driven)
	compressionKeepLast := cfg.Agent.CompressionKeepLast
	if compressionKeepLast <= 0 {
		compressionKeepLast = 10
	}
	compressionMinMessages := cfg.Agent.CompressionMinMessages
	if cfg.Agent.ContinuousCompression {
		hooks.RegisterPriority(core.HookBeforeContextBuild, "continuous-compression", func(ctx context.Context, d *core.HookData) error {
			d.Messages = core.ContinuousCompress(d.Messages, compressionKeepLast, compressionMinMessages)
			return nil
		}, 50)
		log.Printf("[main] continuous compression enabled (keepLast: %d)", compressionKeepLast)
	}

	// Register zone budgeting hook (optional, requires continuousCompression)
	if cfg.Agent.ZoneBudgeting {
		if !cfg.Agent.ContinuousCompression {
			log.Printf("[main] WARNING: zoneBudgeting requires continuousCompression — enabling it")
			hooks.RegisterPriority(core.HookBeforeContextBuild, "continuous-compression", func(ctx context.Context, d *core.HookData) error {
				d.Messages = core.ContinuousCompress(d.Messages, compressionKeepLast, compressionMinMessages)
				return nil
			}, 50)
		}
		archivePercent := cfg.Agent.ZoneArchivePercent
		if archivePercent <= 0 {
			archivePercent = 30
		}
		zoneBudget := core.ZoneBudget{
			ContextWindow:  cfg.Agent.ContextWindow,
			ArchivePercent: archivePercent,
			OutputReserve:  cfg.Agent.MaxTokens,
		}
		hooks.RegisterPriority(core.HookBeforeContextBuild, "zone-budgeting", func(ctx context.Context, d *core.HookData) error {
			d.Messages = core.ApplyZoneBudget(d.Messages, zoneBudget)
			return nil
		}, 60)
		log.Printf("[main] zone budgeting enabled (archive: %d%%, output reserve: %d)", archivePercent, cfg.Agent.MaxTokens)
	}

	// Build tools: default 6 + MCP tools
	defaultTools := tools.BuildDefaultTools()

	// Load MCP servers
	var mcpClient *features.MCPClient
	if len(cfg.MCPServers) > 0 {
		mcpClient = features.NewMCPClient(true) // progressive disclosure
		for name, srv := range cfg.MCPServers {
			if err := mcpClient.AddServer(name, srv.Command, srv.Args, srv.Env); err != nil {
				log.Printf("[mcp] failed to add server %s: %v", name, err)
			} else {
				log.Printf("[mcp] connected to %s", name)
			}
		}
		defaultTools = append(defaultTools, mcpClient.AsTools()...)
		defer mcpClient.Close()
	}

	// Load skills
	skillsDir := cfg.SkillsDir
	if skillsDir == "" {
		skillsDir = filepath.Join(cfgDir, "skills")
	}
	skillRegistry := features.NewSkillRegistry(skillsDir)

	// Create sub-agent manager
	subMgr := features.NewSubAgentManager()

	// Create agent
	agent := core.NewAgent(core.AgentConfig{
		Provider: core.ProviderConfig{
			Name:      cfg.Agent.Provider,
			Model:     cfg.Agent.Model,
			APIKey:    key,
			MaxTokens: cfg.Agent.MaxTokens,
		},
		SystemPrompt:      soul,
		Tools:             defaultTools,
		MaxTurns:          30,
		ContextWindow:     cfg.Agent.ContextWindow,
		SmartRouting:      cfg.Agent.SmartRouting,
		SmartRoutingModel: cfg.Agent.SmartRoutingModel,
	}, prov, hooks, dag)

	// Wire smart routing if configured
	if cfg.Agent.SmartRouting && cfg.Agent.SmartRoutingModel != "" {
		smartProv := makeProvider(cfg.Agent.Provider, key, cfg.Agent.SmartRoutingModel)
		agent.RouteProvider = func(userMessage string) core.Provider {
			if features.IsSimpleMessage(userMessage) {
				return smartProv
			}
			return prov
		}
		log.Printf("[main] smart routing enabled: simple → %s", cfg.Agent.SmartRoutingModel)
	}

	// Override compaction settings from config
	{
		compCfg := agent.GetCompaction()
		switch cfg.Agent.Compaction {
		case "sliding":
			compCfg.Mode = core.CompactionSliding
		case "off":
			compCfg.Mode = core.CompactionOff
		}
		if cfg.Agent.CompactionTrigger != "" {
			compCfg.Trigger = cfg.Agent.CompactionTrigger
		}
		if cfg.Agent.CompactionMaxMessages > 0 {
			compCfg.MaxMessages = cfg.Agent.CompactionMaxMessages
		}
		if cfg.Agent.CompactionThreshold > 0 {
			compCfg.Threshold = cfg.Agent.CompactionThreshold
		}
		if cfg.Agent.CompactionKeepLastN > 0 {
			compCfg.KeepLastN = cfg.Agent.CompactionKeepLastN
		}
		agent.SetCompaction(compCfg)
	}

	if cfg.Agent.SteeringMode != "" {
		agent.SetSteeringMode(cfg.Agent.SteeringMode)
	}

	// Add recall_branch tool — search across all DAG branches
	agent.AddTool(core.Tool{
		Name:        "recall_branch",
		Description: "Search all conversation branches for messages matching a query. Returns relevant excerpts from any branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Text to search for across all branches"},
				"limit": map[string]any{"type": "number", "description": "Max results (default 5)"},
			},
			"required": []string{"query"},
		},
		Execute: func(args map[string]any) (*core.ToolResult, error) {
			query, _ := args["query"].(string)
			limit := int(tools.GF(args, "limit", 5))
			rows, err := dag.SearchAll(query, limit)
			if err != nil {
				return &core.ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
			}
			if rows == "" {
				return &core.ToolResult{Content: "(no matches across any branch)"}, nil
			}
			return &core.ToolResult{Content: rows}, nil
		},
	})

	// Set up compaction summarize callback — use compactionModel if set, otherwise session's model
	compactProv := prov
	if cfg.Agent.CompactionModel != "" {
		compactProv = makeProvider(cfg.Agent.Provider, key, cfg.Agent.CompactionModel)
		log.Printf("[main] compaction model: %s", cfg.Agent.CompactionModel)
	}
	agent.Summarize = func(keyContent string) (string, error) {
		prompt := "Summarize this conversation concisely, preserving key decisions, tool results, and context needed to continue:\n\n" + keyContent
		msgs := []core.Message{{Role: core.RoleUser, Content: []core.ContentBlock{{Type: "text", Text: prompt}}}}
		resp, err := compactProv.Complete(context.Background(), "You are a conversation summarizer. Be concise.", msgs, nil, 2048)
		if err != nil {
			return "", err
		}
		return core.ExtractText(resp), nil
	}

	// Add spawn tool (needs agent reference, so added after agent creation)
	agent.AddTool(core.Tool{
		Name:        "spawn",
		Description: "Spawn a sub-agent to work on a task in the background. Types: builder, researcher, tester.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":       map[string]any{"type": "string", "description": "Task for the sub-agent"},
				"agent_type": map[string]any{"type": "string", "description": "builder, researcher, or tester"},
			},
			"required": []string{"task", "agent_type"},
		},
		Execute: func(args map[string]any) (*core.ToolResult, error) {
			task, _ := args["task"].(string)
			agentType, _ := args["agent_type"].(string)
			id, err := subMgr.SpawnWithProvider(agent, prov, soul, features.SubAgentConfig{
				Task:      task,
				AgentType: agentType,
				Tools:     features.DefaultToolsForType(agentType),
				MaxTurns:  20,
			})
			if err != nil {
				return &core.ToolResult{Content: "Spawn failed: " + err.Error(), IsError: true}, nil
			}
			return &core.ToolResult{Content: fmt.Sprintf("Sub-agent spawned: %s (type: %s)", id, agentType)}, nil
		},
	})

	// Add delegate tool — synchronous agent-as-tool (LLM calls another agent, waits for result)
	agent.AddTool(core.Tool{
		Name:        "delegate",
		Description: "Delegate a task to a sub-agent and wait for the result. Use for tasks that need focused work. Types: builder, researcher, tester.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":       map[string]any{"type": "string", "description": "Task for the sub-agent"},
				"agent_type": map[string]any{"type": "string", "description": "builder, researcher, or tester"},
			},
			"required": []string{"task", "agent_type"},
		},
		Execute: func(args map[string]any) (*core.ToolResult, error) {
			task, _ := args["task"].(string)
			agentType, _ := args["agent_type"].(string)
			id, err := subMgr.SpawnWithProvider(agent, prov, soul, features.SubAgentConfig{
				Task:      task,
				AgentType: agentType,
				Tools:     features.DefaultToolsForType(agentType),
				MaxTurns:  20,
			})
			if err != nil {
				return &core.ToolResult{Content: "Delegate failed: " + err.Error(), IsError: true}, nil
			}
			result := subMgr.Wait(id)
			if result.Error != nil {
				return &core.ToolResult{Content: "Sub-agent error: " + result.Error.Error(), IsError: true}, nil
			}
			return &core.ToolResult{Content: result.Text}, nil
		},
	})

	// Connect DAG to hooks for mutation events
	dag.SetHooks(hooks)

	// Fire app start
	hooks.Fire(context.Background(), core.HookOnAppStart, &core.HookData{
		AgentID: "main",
		Meta:    map[string]any{"provider": cfg.Agent.Provider, "model": cfg.Agent.Model},
	})

	// Select channel: --telegram flag or default to TUI
	channelName := "tui"
	for _, arg := range os.Args[1:] {
		if arg == "--telegram" {
			channelName = "telegram"
		}
	}

	// Pass extras to TUI channel for /stats, /agents, /mcp-tools
	tuichan.Extras = &ui.TUIExtras{
		Telemetry: telemetry,
		SubMgr:    subMgr,
		MCPClient: mcpClient,
	}

	ch, err := channels.Get(channelName)
	if err != nil {
		log.Fatalf("channel: %v", err)
	}
	if err := ch.Start(agent, *cfg, skillRegistry); err != nil {
		log.Fatalf("%s: %v", channelName, err)
	}

	// Fire app shutdown
	hooks.Fire(context.Background(), core.HookOnAppShutdown, &core.HookData{
		AgentID: "main",
	})
	log.Printf("[telemetry] %s", telemetry.Summary())
	_ = subMgr // keep reference alive
}

// agentCfg is set from main() so makeProvider can access Azure/Vertex fields.
var agentCfg *config.AgentConfig

// makeProvider creates a Provider for the given provider name, API key, and model.
func makeProvider(providerName, apiKey, model string) providers.Provider {
	switch strings.ToLower(providerName) {
	case "anthropic":
		return providers.NewAnthropicProvider(apiKey, model)
	case "nvidia":
		return providers.NewNvidiaProvider(apiKey, model)
	case "openai":
		return providers.NewOpenAIProvider(apiKey, model)
	case "grok":
		return providers.NewGrokProvider(apiKey, model)
	case "gemini":
		return providers.NewGeminiProvider(apiKey, model)
	case "azure":
		return providers.NewAzureOpenAIProvider(apiKey, agentCfg.AzureResource, agentCfg.AzureDeployment, agentCfg.AzureAPIVersion)
	case "vertex":
		region := "us-central1"
		if agentCfg != nil && agentCfg.VertexRegion != "" {
			region = agentCfg.VertexRegion
		}
		project := ""
		if agentCfg != nil {
			project = agentCfg.VertexProject
		}
		return providers.NewVertexAIProvider(apiKey, project, region, model)
	default:
		return providers.NewOpenRouterProvider(apiKey, model)
	}
}
