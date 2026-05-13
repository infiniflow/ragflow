//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package security

// ThreatPattern represents a security threat detection pattern
// Inspired by hermes-agent's skills_guard.py
 type ThreatPattern struct {
	Pattern     string // Regular expression pattern
	PatternID   string // Unique identifier for this pattern
	Severity    string // critical | high | medium | low
	Category    string // exfiltration | injection | destructive | persistence | network | obfuscation
	Description string // Human-readable description
}

// ThreatPatterns contains all security threat detection rules
var ThreatPatterns = []ThreatPattern{
	// ========== Data Exfiltration ==========
	{
		Pattern:     `curl\s+[^\n]*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|API)`,
		PatternID:   "env_exfil_curl",
		Severity:    "critical",
		Category:    "exfiltration",
		Description: "curl command interpolating secret environment variable",
	},
	{
		Pattern:     `wget\s+[^\n]*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|API)`,
		PatternID:   "env_exfil_wget",
		Severity:    "critical",
		Category:    "exfiltration",
		Description: "wget command interpolating secret environment variable",
	},
	{
		Pattern:     `\$HOME/\.ssh|\~/\.ssh`,
		PatternID:   "ssh_dir_access",
		Severity:    "high",
		Category:    "exfiltration",
		Description: "references user SSH directory",
	},
	{
		Pattern:     `os\.environ\b`,
		PatternID:   "python_os_environ",
		Severity:    "high",
		Category:    "exfiltration",
		Description: "accesses os.environ (potential env dump)",
	},
	{
		Pattern:     `printenv|env\s*\|`,
		PatternID:   "dump_all_env",
		Severity:    "high",
		Category:    "exfiltration",
		Description: "dumps all environment variables",
	},

	// ========== Prompt Injection ==========
	{
		Pattern:     `(?i)ignore\s+(?:\w+\s+)*(previous|all|above|prior)\s+instructions`,
		PatternID:   "prompt_injection_ignore",
		Severity:    "critical",
		Category:    "injection",
		Description: "prompt injection: ignore previous instructions",
	},
	{
		Pattern:     `(?i)\bDAN\s+mode\b|Do\s+Anything\s+Now`,
		PatternID:   "jailbreak_dan",
		Severity:    "critical",
		Category:    "injection",
		Description: "DAN (Do Anything Now) jailbreak attempt",
	},
	{
		Pattern:     `(?i)you\s+are\s+(?:\w+\s+)*now\s+`,
		PatternID:   "role_hijack",
		Severity:    "high",
		Category:    "injection",
		Description: "attempts to override the agent's role",
	},
	{
		Pattern:     `(?i)system\s+prompt\s+override`,
		PatternID:   "sys_prompt_override",
		Severity:    "critical",
		Category:    "injection",
		Description: "attempts to override the system prompt",
	},
	{
		Pattern:     `(?i)disregard\s+(?:\w+\s+)*(your|all|any)\s+(?:\w+\s+)*(instructions|rules|guidelines)`,
		PatternID:   "disregard_rules",
		Severity:    "critical",
		Category:    "injection",
		Description: "instructs agent to disregard its rules",
	},

	// ========== Destructive Operations ==========
	{
		Pattern:     `rm\s+-rf\s+/`,
		PatternID:   "destructive_root_rm",
		Severity:    "critical",
		Category:    "destructive",
		Description: "recursive delete from root",
	},
	{
		Pattern:     `rm\s+(-[^\s]*)?r.*\$HOME|\brmdir\s+.*\$HOME`,
		PatternID:   "destructive_home_rm",
		Severity:    "critical",
		Category:    "destructive",
		Description: "recursive delete targeting home directory",
	},
	{
		Pattern:     `\bmkfs\b`,
		PatternID:   "format_filesystem",
		Severity:    "critical",
		Category:    "destructive",
		Description: "formats a filesystem",
	},
	{
		Pattern:     `\bdd\s+.*if=.*of=/dev/`,
		PatternID:   "disk_overwrite",
		Severity:    "critical",
		Category:    "destructive",
		Description: "raw disk write operation",
	},
	{
		Pattern:     `shutil\.rmtree\s*\(\s*["\'/]`,
		PatternID:   "python_rmtree",
		Severity:    "high",
		Category:    "destructive",
		Description: "Python rmtree on absolute or root-relative path",
	},
	{
		Pattern:     `rm\s+(-[a-zA-Z]*r[a-zA-Z]*\s+|--)recursive\s+).*\$`,
		PatternID:   "rm_recursive_dangerous",
		Severity:    "high",
		Category:    "destructive",
		Description: "recursive rm with suspicious target",
	},

	// ========== Persistence ==========
	{
		Pattern:     `\bcrontab\b`,
		PatternID:   "persistence_cron",
		Severity:    "medium",
		Category:    "persistence",
		Description: "modifies cron jobs",
	},
	{
		Pattern:     `\.(bashrc|zshrc|profile|bash_profile|bash_login|zprofile|zlogin)\b`,
		PatternID:   "shell_rc_mod",
		Severity:    "medium",
		Category:    "persistence",
		Description: "references shell startup file",
	},
	{
		Pattern:     `authorized_keys`,
		PatternID:   "ssh_backdoor",
		Severity:    "critical",
		Category:    "persistence",
		Description: "modifies SSH authorized keys",
	},
	{
		Pattern:     `AGENTS\.md|CLAUDE\.md|\.cursorrules|\.clinerules`,
		PatternID:   "agent_config_mod",
		Severity:    "critical",
		Category:    "persistence",
		Description: "references agent config files (could persist malicious instructions)",
	},
	{
		Pattern:     `\.ssh/config`,
		PatternID:   "ssh_config_mod",
		Severity:    "high",
		Category:    "persistence",
		Description: "modifies SSH configuration",
	},

	// ========== Network Threats ==========
	{
		Pattern:     `\bnc\s+-[lp]|ncat\s+-[lp]|\bsocat\b`,
		PatternID:   "reverse_shell",
		Severity:    "critical",
		Category:    "network",
		Description: "potential reverse shell listener",
	},
	{
		Pattern:     `/bin/(ba)?sh\s+-i\s+.*>/dev/tcp/`,
		PatternID:   "bash_reverse_shell",
		Severity:    "critical",
		Category:    "network",
		Description: "bash interactive reverse shell via /dev/tcp",
	},
	{
		Pattern:     `\bngrok\b|\blocaltunnel\b|\bserveo\b|\bcloudflared\b`,
		PatternID:   "tunnel_service",
		Severity:    "high",
		Category:    "network",
		Description: "uses tunneling service for external access",
	},
	{
		Pattern:     `webhook\.site|requestbin\.com|pipedream\.net|hookbin\.com`,
		PatternID:   "exfil_service",
		Severity:    "high",
		Category:    "network",
		Description: "references known data exfiltration/webhook testing service",
	},
	{
		Pattern:     `python\s+-c\s+.*socket.*subprocess`,
		PatternID:   "python_reverse_shell",
		Severity:    "critical",
		Category:    "network",
		Description: "Python reverse shell pattern",
	},

	// ========== Obfuscation ==========
	{
		Pattern:     `base64\s+(-d|--decode)\s*\|`,
		PatternID:   "base64_decode_pipe",
		Severity:    "high",
		Category:    "obfuscation",
		Description: "base64 decodes and pipes to execution",
	},
	{
		Pattern:     `\beval\s*\(\s*["\']`,
		PatternID:   "eval_string",
		Severity:    "high",
		Category:    "obfuscation",
		Description: "eval() with string argument",
	},
	{
		Pattern:     `echo\s+[^\n]*\|\s*(bash|sh|python|perl|ruby|node)`,
		PatternID:   "echo_pipe_exec",
		Severity:    "critical",
		Category:    "obfuscation",
		Description: "echo piped to interpreter for execution",
	},
	{
		Pattern:     `curl\s+[^\n]*\|\s*(ba)?sh`,
		PatternID:   "curl_pipe_shell",
		Severity:    "critical",
		Category:    "supply_chain",
		Description: "curl piped to shell (download-and-execute)",
	},
	{
		Pattern:     `\bexec\s*\(\s*(base64|decode|unescape)`,
		PatternID:   "exec_encoded",
		Severity:    "high",
		Category:    "obfuscation",
		Description: "executes encoded content",
	},
}

// TrustedRepos contains the list of trusted repositories
// These repos have a higher trust level
var TrustedRepos = map[string]bool{
	"openai/skills":     true,
	"anthropics/skills": true,
	"microsoft/skills":  true,
	"google/skills":     true,
}

// InstallPolicy defines the installation policy for each trust level
// Format: [safe, caution, dangerous] -> action
// Actions: allow, block, ask
var InstallPolicy = map[string][3]string{
	"builtin":   {"allow", "allow", "allow"},    // Official skills: always allow
	"trusted":   {"allow", "allow", "block"},    // Trusted repos: caution allowed, dangerous blocked
	"community": {"allow", "block", "block"},    // Community: only safe allowed
}

// VerdictIndex maps verdict to array index
var VerdictIndex = map[string]int{
	"safe":      0,
	"caution":   1,
	"dangerous": 2,
}
