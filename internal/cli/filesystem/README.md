# ContextEngine Filesystem

The ContextEngine Filesystem is a filesystem interface for RAGFlow, providing users with a Unix-like file system interface to manage datasets, tools, skills, and memories.

## Directory Structure

```
user_id/
├── datasets/
│   └── my_dataset/
│       └── ...
├── tools/
│   ├── registry.json
│   └── tool_name/
│       ├── DOC.md
│       └── ...
├── skills/
│   └── skill_name/
│       └── version
.          ├──SKILL.md
.          └── ...
└── memories/
    └── memory_id/
        ├── sessions/
        │   ├── messages/
        │   ├── summaries/
        │   │   └── session_id/
        │   │       └── summary-{datetime}.md
        │   └── tools/
        │       └── session_id/
        │           └── {tool_name}.md          # User level of memory on Tools usage
        ├── users/
        │   ├── profile.md
        │   ├── preferences/
        │   └── entities/
        └── agents/
            └── agent_space/
                ├── tools/
                │   └── {tool_name}.md          # Agent level of memory on Tools usage
                └── skills/
                    └── {skill_name}.md         # Agent level of memory on Skills usage
```


## Supported Commands

- `ls [path]` - List directory contents
- `cat <path>` - Display file contents(only for text files)
- `search <query> path` - Search content
- `install-skill <space> <source> [options]` - Install a skill from multiple sources
- `uninstall-skill <space> <skill-name>` - Uninstall a skill

### Skill Management Commands

#### install-skill

Install a skill from multiple sources into a RAGFlow space.

**Usage:**
```bash
install-skill <space> <source> [options]
```

**Arguments:**
- `<space>` - Target skills space ID (required)
- `<source>` - Skill source reference (required)

**Supported Sources:**

| Source Type | Format | Example |
|------------|--------|---------|
| **Local** | `./path` or `/absolute/path` | `./my-skill`, `/home/user/skills/awesome` |
| **GitHub** | `github.com/owner/repo/path` | `github.com/openai/skills/skill-creator` |
| **ClawHub** | `clawhub://owner/skill-name` or `clawhub.ai/owner/skill-name` | `clawhub://pskoett/self-improving-agent` |
| **skills.sh** | `skill://skill-name` or `skills.sh/skill/name` | `skill://kubernetes` |

**Options:**
- `-v, --version <version>` - Specify skill version (default: from SKILL.md or 1.0.0)
- `-n, --name <name>` - Override skill name (default: from SKILL.md)
- `-f, --force` - Force reinstall if skill exists (deletes existing first and updates index)
- `--skip-verify` - Skip security verification (use with caution)
- `-h, --help` - Show help message

**Security Scanning:**

By default, all skills are scanned for potential security threats:
- **Data exfiltration**: Environment variable access, secret leakage, `.ssh` access
- **Prompt injection**: DAN mode, instruction override attempts, role hijacking
- **Destructive commands**: `rm -rf /`, `mkfs`, disk overwrite operations
- **Persistence mechanisms**: Cron jobs, shell RC modification, SSH backdoors
- **Network threats**: Reverse shells, tunneling services, exfiltration endpoints
- **Obfuscation**: Base64 piped to shell, `eval()` usage, encoded execution

**Trust Levels:**
- `builtin` - Official RAGFlow skills (always allowed)
- `trusted` - `openai/skills`, `anthropics/skills`, `microsoft/skills`, `google/skills` (caution allowed)
- `community` - All other sources (findings blocked unless `--force`)

**Examples:**
```bash
# Install from local path
install-skill my-space ./my-local-skill

# Install from GitHub
install-skill my-space github.com/openai/skills/skill-creator

# Install from ClawHub
install-skill my-space clawhub://user/web-search

# Install from Skills.sh
install-skill my-space skills.sh/xixu-me/skills/readme-i18n

# Force reinstall (delete existing and reinstall, update index)
install-skill my-space ./my-skill --force

# Force install with custom name, skip security check
install-skill my-space clawhub://unknown-skill --force --name my-skill --skip-verify

# Install specific version
install-skill my-space skill://kubernetes --version 2.1.0
```

#### uninstall-skill

Remove a skill from RAGFlow and delete its search index.

**Usage:**
```bash
uninstall-skill <space> <skill-name>
```

**Arguments:**
- `<space>` - Skills space ID (required)
- `<skill-name>` - Name of the skill to uninstall (required)

**Examples:**
```bash
uninstall-skill my-space my-skill
```

#### Deprecated Commands

- `add-skill` - Deprecated, use `install-skill` instead
- `delete-skill` - Deprecated, use `uninstall-skill` instead

## File Structure Requirements

### Skill Directory

A valid skill directory must contain:
- `SKILL.md` - Required. Skill metadata and instructions in YAML frontmatter format

Optional files:
- Additional documentation (`.md`, `.mdx`)
- Code files (`.py`, `.js`, `.ts`, etc.)
- Configuration files (`.json`, `.yaml`, `.toml`)

### SKILL.md Frontmatter

```yaml
---
name: my-skill
description: A brief description of what this skill does
version: 1.0.0
author: Your Name
tags:
  - category1
  - category2
---
```

## Security Architecture

The skill management system implements defense-in-depth security:

1. **Source Validation**: All remote sources use HTTPS and verify SSL certificates
2. **Quarantine**: Downloaded skills are isolated before installation
3. **Static Analysis**: Regex-based scanning for 100+ threat patterns across 6 categories:
   - Exfiltration: Environment variable access, secret leakage
   - Injection: Prompt injection, jailbreak attempts
   - Destructive: Dangerous filesystem operations
   - Persistence: Backdoors, startup file modification
   - Network: Reverse shells, unauthorized tunneling
   - Obfuscation: Encoded execution, download-and-run
4. **Trust Tiers**: Different security policies based on source reputation
5. **User Confirmation**: High-risk installations require explicit `--force`
6. **Audit Logging**: All installations are logged with scan results

## Validation Rules

- Total size must not exceed 50MB
- Individual files must not exceed 5MB
- Only text files are allowed (no binaries)
- Skill name must be lowercase alphanumeric with hyphens/underscores
- Hidden files and directories are ignored
