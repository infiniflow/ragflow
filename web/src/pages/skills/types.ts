// Skill types for Skills Hub

// ============================================================================
// Core Skill Types
// ============================================================================

export interface Skill {
  id: string; // Skill name (used as identifier, consistent with search results)
  name: string;
  description: string;
  source_type: 'local' | 'git' | 'central' | 'search';
  source_ref?: string;
  central_path?: string;
  created_at: number;
  updated_at: number;
  files: SkillFileEntry[];
  metadata?: SkillMetadata;
  versions?: string[]; // Available versions (for versioned skills)
  _folderId?: string; // Internal: file system folder ID for file operations
}

export interface SkillsHub {
  id: string;
  name: string;
  folder_id?: string;
  create_time?: number;
}

export interface SkillFileEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  content?: string;
  contentType?: string;
}

// ============================================================================
// Skill Metadata Types
// ============================================================================

export interface SkillMetadata {
  // Basic fields
  name?: string;
  description?: string;
  version?: string;
  author?: string;
  tags?: string[];
  tools?: string[];

  // Legacy fields for backward compatibility
  [key: string]: any;
}

// ============================================================================
// API Payload Types
// ============================================================================

export interface SkillUploadPayload {
  name: string;
  description?: string;
  files: { path: string; content: string }[];
}

export interface SkillUpdatePayload {
  id: string;
  description?: string;
  metadata?: SkillMetadata;
}

// ============================================================================
// Validation Types
// ============================================================================

export interface SkillValidationResult {
  valid: boolean;
  error?: string;
  details?: string;
  name?: string;
  description?: string;
}

export interface ValidationError {
  field: string;
  message: string;
}

// ============================================================================
// UI Types
// ============================================================================

export type ViewMode = 'grid' | 'list';

export interface SkillCardProps {
  skill: Skill;
  onView: (skill: Skill) => void;
  onDelete: (skillId: string, skillName: string, folderId?: string) => void;
  formatRelative: (timestamp: number) => string;
}

export interface SkillDetailProps {
  skill: Skill | null;
  open: boolean;
  onClose: () => void;
  getFileContent: (
    skillId: string,
    filePath: string,
    version?: string,
  ) => Promise<string | null>;
  getVersionFiles?: (
    skillId: string,
    version: string,
  ) => Promise<SkillFileEntry[]>;
}

export interface UploadModalProps {
  open: boolean;
  onCancel: () => void;
  onUpload: (name: string, version: string, files: File[]) => Promise<boolean>;
  loading?: boolean;
}

// ============================================================================
// Skill Search Types
// ============================================================================

export interface FieldWeight {
  enabled: boolean;
  weight: number;
}

export interface FieldConfig {
  name: FieldWeight;
  tags: FieldWeight;
  description: FieldWeight;
  content: FieldWeight;
}

export interface SkillSearchConfig {
  id?: string;
  tenant_id?: string;
  embd_id: string;
  vector_similarity_weight: number;
  similarity_threshold: number;
  field_config: FieldConfig;
  rerank_id?: string;
  top_k: number;
}

export interface SkillSearchResult {
  skill_id: string;
  name: string;
  description: string;
  tags: string[];
  score: number;
  bm25_score?: number;
  vector_score?: number;
}

export interface SkillSearchResponse {
  results: SkillSearchResult[];
  total: number;
  query: string;
  search_type: string;
}

export interface SearchConfigModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  config?: SkillSearchConfig;
  onSave: (config: SkillSearchConfig) => Promise<boolean>;
  onReindex?: (embdId: string) => Promise<boolean>;
  loading?: boolean;
}
