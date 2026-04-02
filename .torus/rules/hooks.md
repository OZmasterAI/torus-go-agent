---
paths:
- "internal/core/hooks*"
- "internal/core/loop*"
- "internal/features/subagents*"
---
## Hooks (41 points)

Pipeline hooks run in priority order (lower first). Can observe, block, or transform via HookData.

Lifecycle: on_agent_start, on_agent_end, on_session_start, on_session_end (defer), on_turn_start, on_turn_end, on_app_start, on_app_shutdown
LLM: before_llm_call, after_llm_call
Tools: before_tool_call, after_tool_call, after_tool_result, post_tool_use_failure
Stop: on_stop (Block overrides stop), before_loop_exit, on_stop_failure
Context: before_context_build, after_context_build, on_token_count, pre_compact, post_compact
DAG: before_new_branch, after_new_branch, pre_clear, post_clear, on_node_added, on_branch_switch
Input: on_user_input
Skills: before_skill, after_skill
Subagents: before_spawn, after_spawn, on_subagent_start (AdditionalContext injection), on_subagent_complete
Tasks: on_task_created, on_task_completed
Notifications: on_notification (Agent.Notify method)
Config: on_config_change (Agent.SetConfig method)
Instructions: on_instructions_loaded (fires on prompt reload)
