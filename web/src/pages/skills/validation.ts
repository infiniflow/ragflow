// Skill validation utilities

import type {
  SkillFileEntry,
  SkillMetadata,
  SkillValidationResult,
} from './types';

// ============================================================================
// Text File Validation
// ============================================================================

const TEXT_FILE_EXTENSIONS = [
  'md',
  'mdx',
  'txt',
  'json',
  'json5',
  'yaml',
  'yml',
  'toml',
  'js',
  'cjs',
  'mjs',
  'ts',
  'tsx',
  'jsx',
  'py',
  'sh',
  'rb',
  'go',
  'rs',
  'swift',
  'kt',
  'java',
  'cs',
  'cpp',
  'c',
  'h',
  'hpp',
  'sql',
  'csv',
  'ini',
  'cfg',
  'env',
  'xml',
  'html',
  'css',
  'scss',
  'sass',
  'svg',
] as const;

const TEXT_FILE_EXTENSION_SET = new Set<string>(TEXT_FILE_EXTENSIONS);

const TEXT_CONTENT_TYPES = [
  'application/json',
  'application/xml',
  'application/yaml',
  'application/x-yaml',
  'application/toml',
  'application/javascript',
  'application/typescript',
  'application/markdown',
  'image/svg+xml',
] as const;

const TEXT_CONTENT_TYPE_SET = new Set<string>(TEXT_CONTENT_TYPES);

/**
 * Check if a content type is text-based
 */
export function isTextContentType(contentType: string): boolean {
  if (!contentType) return false;
  const normalized = contentType.split(';', 1)[0]?.trim().toLowerCase() ?? '';
  if (!normalized) return false;
  if (normalized.startsWith('text/')) return true;
  return TEXT_CONTENT_TYPE_SET.has(normalized);
}

/**
 * Check if a file is a text file based on its extension
 */
export function isTextFile(filePath: string, contentType?: string): boolean {
  // Check content type first
  if (contentType && isTextContentType(contentType)) {
    return true;
  }

  // Check extension
  const ext = filePath.split('.').pop()?.toLowerCase() ?? '';
  if (!ext) return false;
  return TEXT_FILE_EXTENSION_SET.has(ext);
}

// ============================================================================
// Path Sanitization
// ============================================================================

/**
 * Sanitize relative path to prevent directory traversal attacks
 */
export function sanitizeRelPath(path: string): string | null {
  const normalized = path.replace(/^\.\/+/, '').replace(/^\/+/, '');
  if (!normalized || normalized.endsWith('/')) return null;
  if (normalized.includes('..') || normalized.includes('\\')) return null;
  return normalized;
}

/**
 * Check if a path is Mac junk file (should be ignored)
 */
export function isMacJunkPath(path: string): boolean {
  const normalized = path.toLowerCase();
  return (
    normalized.startsWith('__macosx/') ||
    normalized === '.ds_store' ||
    normalized.endsWith('/.ds_store') ||
    normalized.startsWith('._')
  );
}

// ============================================================================
// SKILL.md Validation
// ============================================================================

/**
 * Parse YAML frontmatter from markdown content
 * Returns metadata and body content
 */
export function parseFrontmatter(content: string): {
  metadata: SkillMetadata;
  body: string;
  valid: boolean;
  error?: string;
} {
  const lines = content.split('\n');
  const metadata: SkillMetadata = {};

  // Check frontmatter start
  if (lines[0]?.trim() !== '---') {
    return {
      metadata,
      body: content,
      valid: false,
      error: 'invalid_frontmatter',
    };
  }

  // Find end of frontmatter
  const endIndex = lines.slice(1).findIndex((line) => line.trim() === '---');
  if (endIndex === -1) {
    return {
      metadata,
      body: content,
      valid: false,
      error: 'invalid_frontmatter',
    };
  }

  const metaLines = lines.slice(1, endIndex + 1);
  const body = lines.slice(endIndex + 2).join('\n');

  // Parse YAML-like format
  let currentKey = '';
  let currentIndent = 0;

  for (const line of metaLines) {
    if (!line.trim() || line.trim().startsWith('#')) continue;

    const indent = line.search(/\S/);
    const trimmedLine = line.trim();

    // Handle nested objects (simple implementation)
    const colonMatch = trimmedLine.match(/^(\w+):\s*(.*)$/);
    if (colonMatch) {
      const [, key, value] = colonMatch;
      currentKey = key;
      currentIndent = indent;

      if (value) {
        // Parse value
        metadata[key] = parseYamlValue(value);
      } else {
        // Could be an object or array start
        metadata[key] = {};
      }
    } else if (currentKey && indent > currentIndent) {
      // Nested property
      const nestedMatch = trimmedLine.match(/^(\w+):\s*(.*)$/);
      if (nestedMatch) {
        const [, nestedKey, nestedValue] = nestedMatch;
        if (
          typeof metadata[currentKey] === 'object' &&
          metadata[currentKey] !== null
        ) {
          (metadata[currentKey] as Record<string, unknown>)[nestedKey] =
            parseYamlValue(nestedValue);
        }
      }
    }
  }

  return { metadata, body, valid: true };
}

/**
 * Parse a YAML value string
 */
function parseYamlValue(value: string): unknown {
  const trimmed = value.trim();

  // Boolean
  if (trimmed === 'true') return true;
  if (trimmed === 'false') return false;

  // Null
  if (trimmed === 'null' || trimmed === '~') return null;

  // Number
  if (/^-?\d+$/.test(trimmed)) return parseInt(trimmed, 10);
  if (/^-?\d+\.\d+$/.test(trimmed)) return parseFloat(trimmed);

  // Array
  if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
    return trimmed
      .slice(1, -1)
      .split(',')
      .map((s) => s.trim())
      .filter((s) => s)
      .map(parseYamlValue);
  }

  // Quoted string
  if (
    (trimmed.startsWith('"') && trimmed.endsWith('"')) ||
    (trimmed.startsWith("'") && trimmed.endsWith("'"))
  ) {
    return trimmed.slice(1, -1);
  }

  // Unquoted string
  return trimmed;
}

// ============================================================================
// Main Validation Function
// ============================================================================

const MAX_TOTAL_SIZE = 50 * 1024 * 1024; // 50MB
const MAX_FILE_SIZE = 5 * 1024 * 1024; // 5MB per file

/**
 * Validate skill format
 * This is the main validation function used before upload
 */
export async function validateSkillFormat(
  files: File[],
): Promise<SkillValidationResult> {
  // Check if there are any files
  if (files.length === 0) {
    return { valid: false, error: 'no_files' };
  }

  // Check total size
  const totalSize = files.reduce((sum, f) => sum + f.size, 0);
  if (totalSize > MAX_TOTAL_SIZE) {
    return { valid: false, error: 'total_size_exceeded' };
  }

  // Check individual file sizes
  for (const file of files) {
    if (file.size > MAX_FILE_SIZE) {
      return { valid: false, error: 'file_too_large' };
    }
  }

  // Sanitize and filter paths
  const validFiles: File[] = [];
  for (const file of files) {
    const path = file.webkitRelativePath || file.name;
    const sanitized = sanitizeRelPath(path);

    if (!sanitized) {
      return { valid: false, error: 'invalid_path' };
    }

    if (isMacJunkPath(sanitized)) {
      continue; // Skip Mac junk files
    }

    validFiles.push(file);
  }

  // Find SKILL.md file
  const skillMdFile = validFiles.find((f) => {
    const path = f.webkitRelativePath || f.name;
    const normalized = path.toLowerCase();
    return normalized === 'skill.md' || normalized.endsWith('/skill.md');
  });

  if (!skillMdFile) {
    return { valid: false, error: 'missing_skill_md' };
  }

  // Read and validate SKILL.md content
  try {
    const content = await readFileAsText(skillMdFile);
    const {
      metadata,
      valid: frontmatterValid,
      error: frontmatterError,
    } = parseFrontmatter(content);

    if (!frontmatterValid) {
      return { valid: false, error: frontmatterError || 'invalid_frontmatter' };
    }

    // Validate required fields
    if (!metadata.name) {
      return { valid: false, error: 'missing_name' };
    }

    // Validate name format (slug format: lowercase, URL-safe)
    if (!/^[a-z0-9][a-z0-9_-]*$/.test(metadata.name)) {
      return { valid: false, error: 'invalid_name_format' };
    }

    // Validate version if provided (should be semver)
    if (metadata.version) {
      const version = String(metadata.version);
      // Simple semver check: x.y.z format
      if (!/^\d+\.\d+\.\d+/.test(version)) {
        return { valid: false, error: 'invalid_version' };
      }
    }

    // Validate all files are text-based
    for (const file of validFiles) {
      const path = file.webkitRelativePath || file.name;
      if (!isTextFile(path, file.type)) {
        return { valid: false, error: 'invalid_file_type', details: path };
      }
    }

    return {
      valid: true,
      name: metadata.name,
      description: metadata.description || '',
    };
  } catch (error) {
    console.error('Validation error:', error);
    return { valid: false, error: 'read_failed' };
  }
}

/**
 * Read a File as text
 */
function readFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsText(file);
  });
}

// ============================================================================
// Ignore Pattern Handling (simplified version of ignore package)
// ============================================================================

/**
 * Simple ignore pattern matching
 * Supports basic glob patterns: *, ?, **
 */
export function shouldIgnore(filePath: string, patterns: string[]): boolean {
  for (const pattern of patterns) {
    const trimmedPattern = pattern.trim();
    if (!trimmedPattern || trimmedPattern.startsWith('#')) continue;

    if (matchPattern(filePath, trimmedPattern)) {
      return true;
    }
  }
  return false;
}

function matchPattern(filePath: string, pattern: string): boolean {
  // Handle directory patterns (trailing slash)
  if (pattern.endsWith('/')) {
    const dirPattern = pattern.slice(0, -1);
    return filePath.startsWith(dirPattern + '/') || filePath === dirPattern;
  }

  // Handle exact match
  if (filePath === pattern) return true;

  // Handle glob patterns
  const regex = globToRegex(pattern);
  return regex.test(filePath);
}

function globToRegex(pattern: string): RegExp {
  let regex = '';
  let i = 0;

  while (i < pattern.length) {
    const c = pattern[i];

    if (c === '*') {
      if (pattern[i + 1] === '*') {
        // ** matches any number of directories
        regex += '.*';
        i += 2;
      } else {
        // * matches any characters except /
        regex += '[^/]*';
        i++;
      }
    } else if (c === '?') {
      // ? matches any single character except /
      regex += '[^/]';
      i++;
    } else if (c === '.') {
      regex += '\\.';
      i++;
    } else if (
      c === '\\' ||
      c === '/' ||
      c === '$' ||
      c === '^' ||
      c === '+' ||
      c === '(' ||
      c === ')' ||
      c === '[' ||
      c === ']' ||
      c === '{' ||
      c === '}'
    ) {
      regex += '\\' + c;
      i++;
    } else {
      regex += c;
      i++;
    }
  }

  return new RegExp(`^${regex}$`);
}

// ============================================================================
// Default Ignore Patterns
// ============================================================================

export const DEFAULT_IGNORE_PATTERNS = [
  '.git/',
  'node_modules/',
  '__MACOSX/',
  '.DS_Store',
  '._*',
  '*.log',
  '*.tmp',
  '*.temp',
  '.env',
  '.env.*',
];

// ============================================================================
// File List Filtering
// ============================================================================

/**
 * Filter files based on ignore patterns
 */
export function filterIgnoredFiles(
  files: SkillFileEntry[],
  ignorePatterns: string[] = DEFAULT_IGNORE_PATTERNS,
): SkillFileEntry[] {
  return files.filter((file) => !shouldIgnore(file.path, ignorePatterns));
}

/**
 * Check if a skill folder structure is valid
 */
export function validateSkillStructure(files: SkillFileEntry[]): {
  valid: boolean;
  error?: string;
  skillMdPath?: string;
} {
  // Find SKILL.md
  const skillMdFile = files.find((f) => {
    const normalized = f.path.toLowerCase();
    return normalized === 'skill.md' || normalized.endsWith('/skill.md');
  });

  if (!skillMdFile) {
    return { valid: false, error: 'missing_skill_md' };
  }

  return { valid: true, skillMdPath: skillMdFile.path };
}
