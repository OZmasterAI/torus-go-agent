---
paths:
- "internal/features/skills*"
- "internal/commands/*"
---
## Skills

SkillRegistry loads .md files from a skills directory. Each file becomes a /command.

- Filename (minus .md) = command name
- First line = description (shown in /skills listing)
- Full content = prompt injected when user invokes /command
- NewSkillRegistry(dir) auto-loads on creation
- Load() rescans directory (supports hot-reload)

Skills hook into commands.go via /skills list and /<name> dispatch.
