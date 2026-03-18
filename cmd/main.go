package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"go_sdk_agent/internal/channels"
	_ "go_sdk_agent/internal/channels/http"     // register http channel
	_ "go_sdk_agent/internal/channels/telegram" // register telegram channel
	_ "go_sdk_agent/internal/channels/tui"      // register tui channel
	"go_sdk_agent/internal/config"
	"go_sdk_agent/internal/core"
	"go_sdk_agent/internal/features"
	"go_sdk_agent/internal/providers"
	"go_sdk_agent/internal/ui"
)

func main() {
	_ = godotenv.Load() // load .env if present
	cfgDir := filepath.Join(".", "config")
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
	if startProvider, startModel := ui.RunStartup(skipSetup); startProvider != "" {
		cfg.Agent.Provider = startProvider
		cfg.Agent.Model = startModel
	}

	soul := config.LoadTorus(cfgDir)
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
	var prov providers.Provider
	switch cfg.Agent.Provider {
	case "anthropic":
		prov = providers.NewAnthropicProvider(key, cfg.Agent.Model)
	case "nvidia":
		prov = providers.NewNvidiaProvider(key, cfg.Agent.Model)
	default:
		prov = providers.NewOpenRouterProvider(key, cfg.Agent.Model)
	}

	// Create DAG
	dataDir := cfg.DataDir(cfgDir)
	os.MkdirAll(dataDir, 0755)
	dag, err := core.NewDAG(filepath.Join(dataDir, "conversations.db"))
	if err != nil {
		log.Fatalf("dag: %v", err)
	}
	defer dag.Close()

	// Create hooks + register secret scanner
	hooks := core.NewHookRegistry()
	hooks.Register(core.HookBeforeToolCall, "secret-scan", func(ctx context.Context, d *core.HookData) error {
		if d.ToolName == "write" || d.ToolName == "edit" {
			content, _ := d.ToolArgs["content"].(string)
			if content == "" {
				content, _ = d.ToolArgs["new_str"].(string)
			}
			if desc, found := core.ScanSecrets(content); found {
				d.Block = true
				d.BlockReason = "Secret detected: " + desc
			}
		}
		return nil
	})

	// Inject DAG schema + live branch state so the LLM knows the active branch.
	dagSchemaBase := "[SYSTEM: DAG schema] tables: nodes(id, parent_id, role, content, model, provider, timestamp, token_count), branches(id, name, head_node_id, forked_from). DB path: " + filepath.Join(dataDir, "conversations.db")
	hooks.Register(core.HookBeforeContextBuild, "dag-context", func(ctx context.Context, d *core.HookData) error {
		brID, brName, headNode, msgCount := dag.CurrentBranchInfo()
		contextLine := fmt.Sprintf("[SYSTEM: DAG context] active_branch: %s, name: %s, head: %s, messages: %d", brID, brName, headNode, msgCount)
		schema := core.Message{
			Role:    core.RoleUser,
			Content: []core.ContentBlock{{Type: "text", Text: dagSchemaBase + "\n" + contextLine}},
		}
		d.Messages = append([]core.Message{schema}, d.Messages...)
		return nil
	})

	// Build tools: default 6 + MCP tools
	tools := core.BuildDefaultTools()

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
		tools = append(tools, mcpClient.AsTools()...)
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
		SystemPrompt: soul,
		Tools:        tools,
		MaxTurns:     30,
	}, prov, hooks, dag)

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

	// Connect DAG to hooks for mutation events
	dag.SetHooks(hooks)

	// Fire app start
	hooks.Fire(context.Background(), core.HookOnAppStart, &core.HookData{
		AgentID: "main",
		Meta:    map[string]any{"provider": cfg.Agent.Provider, "model": cfg.Agent.Model},
	})

	// Select channel: --telegram/--http flag or default to TUI
	channelName := "tui"
	for _, arg := range os.Args[1:] {
		if arg == "--telegram" {
			channelName = "telegram"
		}
		if arg == "--http" {
			channelName = "http"
		}
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
	_ = subMgr // keep reference alive
}
