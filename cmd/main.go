package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"torus_go_agent/internal/channels"
	batchchan "torus_go_agent/internal/channels/batch"
	_ "torus_go_agent/internal/channels/http"     // register http channel
	_ "torus_go_agent/internal/channels/telegram" // register telegram channel
	tuichan "torus_go_agent/internal/channels/tui"
	tuibchan "torus_go_agent/internal/channels/tui-b"
	"torus_go_agent/internal/config"
	"torus_go_agent/internal/constants"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/features"
	"torus_go_agent/internal/providers"
	"torus_go_agent/internal/safety"
	"torus_go_agent/internal/tools"
	"torus_go_agent/internal/types"
	"torus_go_agent/internal/ui"
	uib "torus_go_agent/internal/ui-b"
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
		os.MkdirAll(d, constants.DirPerm)
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "config")
	}
	d := filepath.Join(home, ".config", "torus_go_agent")
	os.MkdirAll(d, constants.DirPerm)
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
		if arg == "--no-setup" || arg == "--telegram" || arg == "--http" || strings.HasPrefix(arg, "--batch") {
			skipSetup = true
		}
	}
	setup := ui.RunStartup(skipSetup, cfg.Agent)
	if setup.Provider != "" {
		cfg.Agent.Provider = setup.Provider
		cfg.Agent.Model = setup.Model
		// Persist so user doesn't re-pick next run
		cfgPath := filepath.Join(cfgDir, "config.json")
		if err := config.SaveConfig(cfgPath, cfg); err != nil {
			log.Printf("[main] warning: could not save config: %v", err)
		}
	}
	// Require provider and model
	if cfg.Agent.Provider == "" || cfg.Agent.Model == "" {
		fmt.Fprintln(os.Stderr, "AGENT_PROVIDER and AGENT_MODEL must be set (via .env, environment, or startup screen).")
		os.Exit(1)
	}

	// Auto-detect model specs: models.json → OpenRouter API → code defaults
	models := config.LoadModels(cfgDir)
	if info := config.ResolveModelInfo(cfg.Agent.Model, cfg.Agent.Provider, models, cfgDir); info.ContextWindow > 0 {
		// Guard against a nonsensical MaxTokens that meets or exceeds the context
		// window, which would leave no room for input. Pin to a quarter window.
		if info.MaxTokens >= info.ContextWindow {
			info.MaxTokens = info.ContextWindow / 4
		}
		cfg.Agent.ContextWindow = info.ContextWindow
		if info.MaxTokens > 0 {
			cfg.Agent.MaxTokens = info.MaxTokens
		}
		log.Printf("[main] model %s: context=%d, maxTokens=%d (auto-detected)", cfg.Agent.Model, info.ContextWindow, info.MaxTokens)
	} else {
		log.Printf("[main] warning: model %s unresolved; using generic default specs (context=%d, maxTokens=%d)", cfg.Agent.Model, cfg.Agent.ContextWindow, cfg.Agent.MaxTokens)
	}

	// Apply startup screen overrides (user values win over auto-resolved)
	if setup.Config != nil {
		if setup.Config.MaxTokens > 0 {
			cfg.Agent.MaxTokens = setup.Config.MaxTokens
		}
		if setup.Config.ContextWindow > 0 {
			cfg.Agent.ContextWindow = setup.Config.ContextWindow
		}
		cfg.Agent.Compaction = setup.Config.Compaction
		cfg.Agent.CompactionTrigger = setup.Config.CompactionTrigger
		cfg.Agent.CompactionThreshold = setup.Config.CompactionThreshold
		cfg.Agent.CompactionMaxMessages = setup.Config.CompactionMaxMessages
		cfg.Agent.CompactionKeepLastN = setup.Config.CompactionKeepLastN
		cfg.Agent.CompactionModel = setup.Config.CompactionModel
		cfg.Agent.ContinuousCompression = setup.Config.ContinuousCompression
		cfg.Agent.CompressionKeepFirst = setup.Config.CompressionKeepFirst
		cfg.Agent.CompressionKeepLast = setup.Config.CompressionKeepLast
		cfg.Agent.CompressionMinMessages = setup.Config.CompressionMinMessages
		cfg.Agent.ZoneBudgeting = setup.Config.ZoneBudgeting
		cfg.Agent.ZoneArchivePercent = setup.Config.ZoneArchivePercent
		cfg.Agent.SmartRouting = setup.Config.SmartRouting
		cfg.Agent.SmartRoutingModel = setup.Config.SmartRoutingModel
		cfg.Agent.SteeringMode = setup.Config.SteeringMode
		cfg.Agent.PersistThinking = setup.Config.PersistThinking
		cfg.Agent.Thinking = setup.Config.Thinking
		cfg.Agent.ThinkingBudget = setup.Config.ThinkingBudget
		cfg.Agent.RewardScoring = setup.Config.RewardScoring
		cfg.Agent.ForceStream = setup.Config.ForceStream
	}

	// Build system prompt from multi-tier instruction files.
	initCwd, _ := os.Getwd()
	initFiles := core.LoadAndParseAll(initCwd, core.LoadReasonSessionStart)
	soul := core.BuildPrompt(initFiles, nil)
	if soul == "" {
		// Fallback to legacy config/ location if no instruction files discovered.
		soul = config.LoadTorus(cfgDir)
	}
	soul = strings.ReplaceAll(soul, "{{MODEL}}", cfg.Agent.Provider+"/"+cfg.Agent.Model)
	soul = strings.ReplaceAll(soul, "{{CWD}}", initCwd)
	schema := config.LoadSchema(cfgDir)
	schema += config.GenerateFileTree(initCwd, 4)
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
			if err := providers.SaveCredentials(creds); err != nil {
				log.Printf("[oauth] warning: could not persist credentials: %v", err)
			}
			key = creds.Access
			fmt.Println("Login successful.")
		} else {
			key = oauthKey
		}
		// Make OAuth key available for cross-provider Anthropic model selection
		// (e.g. compactionModel or smartRoutingModel using an Anthropic model)
		os.Setenv("ANTHROPIC_API_KEY", key)
	}
	if key == "" && cfg.Agent.Provider == "openai" {
		oauthKey, err := providers.GetOpenAIAccessToken()
		if err != nil {
			fmt.Println("No API key. Starting Sign in with ChatGPT...")
			// Headless mode skips the loopback callback entirely — for remote/VPS
			// boxes where the browser's localhost redirect points at the user's
			// laptop, not the server. LoginOpenAI also auto-falls back to the same
			// paste flow if it cannot bind the loopback port.
			headless := os.Getenv("TORUS_OAUTH_HEADLESS") == "1"
			onAuthURL := func(u string) {
				fmt.Println("\nOpen this URL in a browser to sign in with ChatGPT:\n  " + u)
				fmt.Println("\nOn a remote/VPS/headless machine the page at localhost will not load —")
				fmt.Println("after you approve, copy the full URL from your browser's address bar and paste it here.")
				fmt.Println()
			}
			onPromptCode := func() (string, error) {
				fmt.Print("Paste the redirect URL (or code): ")
				line, err := bufio.NewReader(os.Stdin).ReadString('\n')
				if err != nil && strings.TrimSpace(line) == "" {
					return "", err
				}
				return strings.TrimSpace(line), nil
			}
			var creds *providers.OpenAICredentials
			var loginErr error
			if headless {
				creds, loginErr = providers.LoginOpenAIHeadless(onAuthURL, onPromptCode)
			} else {
				creds, loginErr = providers.LoginOpenAI(onAuthURL, onPromptCode)
			}
			if loginErr != nil {
				log.Fatalf("ChatGPT OAuth failed: %v", loginErr)
			}
			if err := providers.SaveOpenAICredentials(creds); err != nil {
				log.Printf("[openai-oauth] warning: could not persist credentials: %v", err)
			}
			key = creds.Access
			fmt.Println("Login successful (using ChatGPT subscription).")
		} else {
			key = oauthKey
		}
		// Export so secondary providers (routing / fallback / compaction /
		// smart-routing) with provider=openai inherit the ChatGPT token,
		// mirroring the Anthropic path above.
		if key != "" {
			os.Setenv("OPENAI_API_KEY", key)
		}
	}
	if key == "" {
		fmt.Fprintln(os.Stderr, "No API key. Set OPENROUTER_API_KEY or ANTHROPIC_API_KEY, or select the openai provider to Sign in with ChatGPT.")
		os.Exit(1)
	}

	// Create provider
	prov := makeProvider(cfg.Agent.Provider, key, cfg.Agent.Model, &cfg.Agent)

	// Resolve the agent's primary provider and the router that weighted routing /
	// fallback should target. ResolveRouter avoids double-wrapping when prov is
	// already a *Router (nvidia/free) or *RewardRouter — otherwise cfg.Routing /
	// FallbackOrder would attach to an inert outer shell. (audit #4)
	agentProv, router := providers.ResolveRouter(prov)
	if len(cfg.Agent.Routing) > 0 {
		var entries []providers.RoutingEntry
		for _, r := range cfg.Agent.Routing {
			rKey := config.APIKeyFor(r.Provider)
			rProv := makeProvider(r.Provider, rKey, r.Model, &cfg.Agent)
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
	os.MkdirAll(dataDir, constants.DirPerm)
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
		// Scan ALL tool inputs for secrets (covers write, edit, bash, MCP tools, etc.)
		for _, v := range d.ToolArgs {
			s, ok := v.(string)
			if !ok || s == "" {
				continue
			}
			if desc, found := safety.ScanSecrets(s); found {
				d.Block = true
				d.BlockReason = "Secret detected: " + desc
				return nil
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
	// Priority 100 (> compression's 50) so this runs AFTER unified-compression /
	// zone-budget: the live [DAG state] line is appended post-compression and is
	// never itself compressed away. (audit #5)
	hooks.RegisterPriority(core.HookBeforeContextBuild, "dag-context", func(ctx context.Context, d *core.HookData) error {
		brID, brName, headNode, msgCount, err := dag.CurrentBranchInfo()
		if err != nil {
			return nil // skip context injection on error
		}
		contextLine := fmt.Sprintf("[DAG state] branch: %s (%s), head: %s, messages: %d", brID, brName, headNode, msgCount)
		state := types.Message{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{{Type: "text", Text: contextLine}},
		}
		d.Messages = append([]types.Message{state}, d.Messages...)
		return nil
	}, 100)

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
		dag.AddNode("", types.RoleUser, []types.ContentBlock{{Type: "text", Text: schema}}, "", "", 0)
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

	// Register unified compression pipeline (optional, config-driven)
	// Replaces both continuous compression and zone budgeting with a single pass.
	if cfg.Agent.ContinuousCompression {
		archivePct := cfg.Agent.ZoneArchivePercent
		if archivePct <= 0 {
			archivePct = 25
		}
		compressCfg := core.UnifiedCompressConfig{
			KeepFirst:     cfg.Agent.CompressionKeepFirst,
			KeepLast:      cfg.Agent.CompressionKeepLast,
			MinMessages:   cfg.Agent.CompressionMinMessages,
			ContextWindow: cfg.Agent.ContextWindow,
			MaxTokens:     cfg.Agent.MaxTokens,
			ArchivePct:    archivePct,
		}
		hooks.RegisterPriority(core.HookBeforeContextBuild, "unified-compression", func(ctx context.Context, d *core.HookData) error {
			d.Messages = core.UnifiedCompress(d.Messages, compressCfg)
			return nil
		}, 50)
		log.Printf("[main] unified compression enabled (keepFirst: %d, keepLast: %d, archive: %d%%)", compressCfg.KeepFirst, compressCfg.KeepLast, archivePct)
	} else if cfg.Agent.ZoneBudgeting {
		// Legacy zone-budgeting strategy, used only when the unified pipeline is
		// off — so the config comment "applied only when continuousCompression is
		// false" is now accurate instead of the flag being a no-op. (audit #2)
		archivePct := cfg.Agent.ZoneArchivePercent
		if archivePct <= 0 {
			archivePct = 25
		}
		zb := core.ZoneBudgetV2{
			ContextWindow:    cfg.Agent.ContextWindow,
			SystemArchivePct: archivePct,
		}
		hooks.RegisterPriority(core.HookBeforeContextBuild, "zone-budget", func(ctx context.Context, d *core.HookData) error {
			d.Messages = core.ApplyZoneBudgetV2(d.Messages, zb, nil)
			return nil
		}, 50)
		log.Printf("[main] zone budgeting enabled (archive: %d%%)", archivePct)
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
	agent := core.NewAgent(types.AgentConfig{
		Provider: types.ProviderConfig{
			Name:      cfg.Agent.Provider,
			Model:     cfg.Agent.Model,
			APIKey:    key,
			MaxTokens: cfg.Agent.MaxTokens,
		},
		SystemPrompt:      soul,
		Tools:             defaultTools,
		MaxTurns:          types.DefaultMaxTurns,
		ContextWindow:     cfg.Agent.ContextWindow,
		SmartRouting:      cfg.Agent.SmartRouting,
		SmartRoutingModel: cfg.Agent.SmartRoutingModel,
		PersistThinking:   cfg.Agent.PersistThinking,
		ParallelTools:     cfg.Agent.ParallelTools,
		ForceStream:       cfg.Agent.ForceStream,
	}, agentProv, hooks, dag)

	// Hot-reload: watch all discovered instruction files.
	var watchPaths []string
	for _, f := range initFiles {
		watchPaths = append(watchPaths, f.Path)
	}
	if torusPath := filepath.Join(cfgDir, "TORUS.md"); !contains(watchPaths, torusPath) {
		watchPaths = append(watchPaths, torusPath)
	}
	reloader := core.NewPromptReloader(agent, watchPaths, 5*time.Second, func() string {
		files := core.LoadAndParseAll(initCwd, core.LoadReasonFileChange)
		s := core.BuildPrompt(files, agent.ActiveFiles())
		s = strings.ReplaceAll(s, "{{MODEL}}", cfg.Agent.Provider+"/"+cfg.Agent.Model)
		s = strings.ReplaceAll(s, "{{CWD}}", initCwd)
		return s
	})
	reloader.Start()
	defer reloader.Stop()

	// Wire smart routing if configured
	if cfg.Agent.SmartRouting && cfg.Agent.SmartRoutingModel != "" {
		smartProv := resolveProviderModel(cfg.Agent.SmartRoutingModel, cfg.Agent.Provider, key, &cfg.Agent)
		agent.RouteProvider = func(userMessage string) types.Provider {
			if features.IsSimpleMessage(userMessage) {
				return smartProv
			}
			// Complex messages use the agent's full provider (weighted routing +
			// fallback + reward scoring), not the bare base provider. (audit #1)
			return agentProv
		}
		log.Printf("[main] smart routing enabled: simple → %s", cfg.Agent.SmartRoutingModel)
	}

	// Override compaction settings from config
	{
		compCfg := agent.GetCompaction()
		switch cfg.Agent.Compaction {
		case "sliding":
			compCfg.Mode = core.CompactionSliding
		case "llm":
			compCfg.Mode = core.CompactionLLM
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
	agent.AddTool(types.Tool{
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
		Execute: func(args map[string]any) (*types.ToolResult, error) {
			query, _ := args["query"].(string)
			limit := int(tools.GetFloat(args, "limit", 5))
			rows, err := dag.SearchAll(query, limit)
			if err != nil {
				return &types.ToolResult{Content: "Error: " + err.Error(), IsError: true}, nil
			}
			if rows == "" {
				return &types.ToolResult{Content: "(no matches across any branch)"}, nil
			}
			return &types.ToolResult{Content: rows}, nil
		},
	})

	// Set up compaction summarize callback — use compactionModel if set, otherwise session's model
	compactProv := prov
	if cfg.Agent.CompactionModel != "" {
		compactProv = resolveProviderModel(cfg.Agent.CompactionModel, cfg.Agent.Provider, key, &cfg.Agent)
		log.Printf("[main] compaction model: %s", cfg.Agent.CompactionModel)
	}
	agent.Summarize = func(keyContent string) (string, error) {
		prompt := "Summarize this conversation concisely, preserving key decisions, tool results, and context needed to continue:\n\n" + keyContent
		msgs := []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{{Type: "text", Text: prompt}}}}
		resp, err := compactProv.Complete(context.Background(), "You are a conversation summarizer. Be concise.", msgs, nil, 2048)
		if err != nil {
			return "", err
		}
		return core.ExtractText(resp), nil
	}

	// Add spawn tool (needs agent reference, so added after agent creation)
	agent.AddTool(types.Tool{
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
		Execute: func(args map[string]any) (*types.ToolResult, error) {
			task, _ := args["task"].(string)
			agentType, _ := args["agent_type"].(string)
			id, err := subMgr.SpawnWithProvider(agent, prov, soul, features.SubAgentConfig{
				Task:      task,
				AgentType: agentType,
				Tools:     features.DefaultToolsForType(agentType),
				MaxTurns:  types.SubAgentMaxTurns,
			})
			if err != nil {
				return &types.ToolResult{Content: "Spawn failed: " + err.Error(), IsError: true}, nil
			}
			return &types.ToolResult{Content: fmt.Sprintf("Sub-agent spawned: %s (type: %s)", id, agentType)}, nil
		},
	})

	// Add delegate tool — synchronous agent-as-tool (LLM calls another agent, waits for result)
	agent.AddTool(types.Tool{
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
		Execute: func(args map[string]any) (*types.ToolResult, error) {
			task, _ := args["task"].(string)
			agentType, _ := args["agent_type"].(string)
			id, err := subMgr.SpawnWithProvider(agent, prov, soul, features.SubAgentConfig{
				Task:      task,
				AgentType: agentType,
				Tools:     features.DefaultToolsForType(agentType),
				MaxTurns:  types.SubAgentMaxTurns,
			})
			if err != nil {
				return &types.ToolResult{Content: "Delegate failed: " + err.Error(), IsError: true}, nil
			}
			result := subMgr.Wait(id)
			subMgr.DeleteResult(id)
			if result.Error != nil {
				return &types.ToolResult{Content: "Sub-agent error: " + result.Error.Error(), IsError: true}, nil
			}
			return &types.ToolResult{Content: result.Text}, nil
		},
	})

	// Add run_sequential tool — pipeline of agents where each gets the previous output
	agent.AddTool(types.Tool{
		Name:        "run_sequential",
		Description: "Run multiple sub-agents in sequence. Each agent receives the previous agent's output as context. Returns the final agent's output.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agents": map[string]any{
					"type":        "array",
					"description": "List of agents to run in order",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"task":       map[string]any{"type": "string", "description": "Task for the agent"},
							"agent_type": map[string]any{"type": "string", "description": "builder, researcher, or tester"},
						},
						"required": []string{"task"},
					},
				},
			},
			"required": []string{"agents"},
		},
		Execute: func(args map[string]any) (*types.ToolResult, error) {
			rawAgents, _ := args["agents"].([]any)
			if len(rawAgents) == 0 {
				return &types.ToolResult{Content: "Error: agents array is empty", IsError: true}, nil
			}
			configs := make([]features.SubAgentConfig, 0, len(rawAgents))
			for _, raw := range rawAgents {
				m, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				task, _ := m["task"].(string)
				agentType, _ := m["agent_type"].(string)
				if agentType == "" {
					agentType = "builder"
				}
				configs = append(configs, features.SubAgentConfig{
					Task:      task,
					AgentType: agentType,
					Tools:     features.DefaultToolsForType(agentType),
					MaxTurns:  types.SubAgentMaxTurns,
				})
			}
			result, err := features.RunSequential(context.Background(), dag, prov, soul, configs, subMgr, agent)
			if err != nil {
				return &types.ToolResult{Content: "Sequential run failed: " + err.Error(), IsError: true}, nil
			}
			return &types.ToolResult{Content: result}, nil
		},
	})

	// Add run_parallel tool — fan-out agents concurrently and collect all results
	agent.AddTool(types.Tool{
		Name:        "run_parallel",
		Description: "Run multiple sub-agents concurrently. All agents run at the same time. Returns all results.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agents": map[string]any{
					"type":        "array",
					"description": "List of agents to run concurrently",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"task":       map[string]any{"type": "string", "description": "Task for the agent"},
							"agent_type": map[string]any{"type": "string", "description": "builder, researcher, or tester"},
						},
						"required": []string{"task"},
					},
				},
			},
			"required": []string{"agents"},
		},
		Execute: func(args map[string]any) (*types.ToolResult, error) {
			rawAgents, _ := args["agents"].([]any)
			if len(rawAgents) == 0 {
				return &types.ToolResult{Content: "Error: agents array is empty", IsError: true}, nil
			}
			configs := make([]features.SubAgentConfig, 0, len(rawAgents))
			for _, raw := range rawAgents {
				m, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				task, _ := m["task"].(string)
				agentType, _ := m["agent_type"].(string)
				if agentType == "" {
					agentType = "builder"
				}
				configs = append(configs, features.SubAgentConfig{
					Task:      task,
					AgentType: agentType,
					Tools:     features.DefaultToolsForType(agentType),
					MaxTurns:  types.SubAgentMaxTurns,
				})
			}
			results, err := features.RunParallel(context.Background(), dag, prov, soul, configs, subMgr, agent)
			if err != nil {
				return &types.ToolResult{Content: "Parallel run failed: " + err.Error(), IsError: true}, nil
			}
			var out strings.Builder
			for i, r := range results {
				fmt.Fprintf(&out, "=== Agent %d ===\n", i+1)
				if r.Error != nil {
					fmt.Fprintf(&out, "Error: %s\n", r.Error.Error())
				} else {
					fmt.Fprintf(&out, "%s\n", r.Text)
				}
			}
			return &types.ToolResult{Content: out.String()}, nil
		},
	})

	// Add run_loop tool — repeat one agent until a condition is met
	agent.AddTool(types.Tool{
		Name:        "run_loop",
		Description: "Run a single sub-agent repeatedly. Each iteration receives the previous output. Stops when max_iterations is reached or the agent's output contains the stop_phrase.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":           map[string]any{"type": "string", "description": "Task for the agent"},
				"agent_type":     map[string]any{"type": "string", "description": "builder, researcher, or tester"},
				"max_iterations": map[string]any{"type": "number", "description": "Maximum iterations (default 5)"},
				"stop_phrase":    map[string]any{"type": "string", "description": "Stop when output contains this phrase (e.g. 'DONE', 'ALL TESTS PASS')"},
			},
			"required": []string{"task"},
		},
		Execute: func(args map[string]any) (*types.ToolResult, error) {
			task, _ := args["task"].(string)
			agentType, _ := args["agent_type"].(string)
			if agentType == "" {
				agentType = "builder"
			}
			maxIter := int(tools.GetFloat(args, "max_iterations", 5))
			stopPhrase, _ := args["stop_phrase"].(string)

			cfg := features.SubAgentConfig{
				Task:      task,
				AgentType: agentType,
				Tools:     features.DefaultToolsForType(agentType),
				MaxTurns:  types.SubAgentMaxTurns,
			}

			shouldStop := func(result string, iteration int) bool {
				return stopPhrase != "" && strings.Contains(result, stopPhrase)
			}

			result, err := features.RunLoop(context.Background(), dag, prov, soul, cfg, subMgr, agent, shouldStop, maxIter)
			if err != nil {
				return &types.ToolResult{Content: "Loop run failed: " + err.Error(), IsError: true}, nil
			}
			return &types.ToolResult{Content: result}, nil
		},
	})

	// Connect DAG to hooks for mutation events
	dag.SetHooks(hooks)

	// Fire app start
	hooks.Fire(context.Background(), core.HookOnAppStart, &core.HookData{
		AgentID: "main",
		Meta:    map[string]any{"provider": cfg.Agent.Provider, "model": cfg.Agent.Model},
	})

	// Select channel: --telegram/--http/--batch flag or default to TUI
	channelName := "tui"
	for _, arg := range os.Args[1:] {
		if arg == "--telegram" {
			channelName = "telegram"
		}
		if arg == "--http" {
			channelName = "http"
		}
		if arg == "--tui-b" {
			channelName = "tui-b"
		}
		if strings.HasPrefix(arg, "--batch") {
			channelName = "batch"
			if strings.HasPrefix(arg, "--batch=") {
				batchchan.Config.PromptFile = arg[8:]
			}
		}
		if strings.HasPrefix(arg, "--output=") {
			batchchan.Config.OutputDir = arg[9:]
		}
		if strings.HasPrefix(arg, "--workdir=") {
			batchchan.Config.WorkDir = arg[10:]
		}
		if arg == "--multi-turn" {
			batchchan.Config.MultiTurn = true
		}
		if strings.HasPrefix(arg, "--context-window=") {
			if v, err := strconv.Atoi(arg[17:]); err == nil && v > 0 {
				cfg.Agent.ContextWindow = v
			}
		}
	}

	// Pass extras to TUI channel for /stats, /agents, /mcp-tools
	tuichan.Extras = &ui.TUIExtras{
		Telemetry: telemetry,
		SubMgr:    subMgr,
		MCPClient: mcpClient,
	}
	tuibchan.Extras = &uib.TUIExtras{
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
}

// resolveProviderModel parses a "provider:model" string and creates the right provider.
// If no ":" is found, uses defaultProvider and defaultKey as fallback.
func resolveProviderModel(providerModel, defaultProvider, defaultKey string, agentCfg *config.AgentConfig) types.Provider {
	prov, model := defaultProvider, providerModel
	if idx := strings.IndexByte(providerModel, ':'); idx > 0 {
		prov = providerModel[:idx]
		model = providerModel[idx+1:]
	}
	k := config.APIKeyFor(prov)
	if k == "" {
		k = defaultKey
	}
	return makeProvider(prov, k, model, agentCfg)
}

// makeProvider creates a Provider for the given provider name, API key, and model.
func makeProvider(providerName, apiKey, model string, agentCfg *config.AgentConfig) types.Provider {
	switch strings.ToLower(providerName) {
	case "anthropic":
		p := providers.NewAnthropicProvider(apiKey, model)
		if agentCfg != nil {
			if agentCfg.BaseURL != "" {
				p.BaseURL = agentCfg.BaseURL
			}
			if agentCfg.ThinkingBudget > 0 {
				p.ThinkingBudget = agentCfg.ThinkingBudget
			} else if agentCfg.Thinking != "" {
				p.ThinkingBudget = providers.ThinkingBudgetForLevel(agentCfg.Thinking)
			}
		}
		return p
	case "nvidia":
		if model == "nvidia/free" {
			router := providers.NewNvidiaFreeRouter(apiKey)
			if agentCfg != nil && agentCfg.RewardScoring {
				return providers.NewRewardRouter(router, apiKey)
			}
			return router
		}
		p := providers.NewNvidiaProvider(apiKey, model)
		if agentCfg != nil && agentCfg.BaseURL != "" {
			p.BaseURL = agentCfg.BaseURL
		}
		return p
	case "openai":
		// A ChatGPT OAuth token (JWT) routes through the subscription backend
		// (Responses API); a standard sk- key uses the API-billed path.
		if providers.IsOpenAIOAuthToken(apiKey) {
			return providers.NewOpenAIChatGPTProvider(apiKey, providers.OpenAIAccountID(), model)
		}
		p := providers.NewOpenAIProvider(apiKey, model)
		if agentCfg != nil && agentCfg.BaseURL != "" {
			p.BaseURL = agentCfg.BaseURL
		}
		return p
	case "grok":
		p := providers.NewGrokProvider(apiKey, model)
		if agentCfg != nil && agentCfg.BaseURL != "" {
			p.BaseURL = agentCfg.BaseURL
		}
		return p
	case "deepseek":
		p := providers.NewDeepSeekProvider(apiKey, model)
		if agentCfg != nil && agentCfg.BaseURL != "" {
			p.BaseURL = agentCfg.BaseURL
		}
		return p
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
		p := providers.NewOpenRouterProvider(apiKey, model)
		if agentCfg != nil && agentCfg.BaseURL != "" {
			p.BaseURL = agentCfg.BaseURL
		}
		return p
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
