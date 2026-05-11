# ContextFS - Context Engine File System

ContextFS is a context engine interface for RAGFlow, providing users with a Unix-like file system interface to manage datasets, tools, skills, and memories.

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
│   ├── registry.json
│   └── skill_name/
│       ├── SKILL.md
│       └── ...
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
- `search <query>` - Search content
