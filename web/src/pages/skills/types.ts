// Skill types for Skills Hub

// ============================================================================
// Core Skill Types
// ============================================================================

export interface Skill {
  id: string;
  name: string;
  description: string;
  source_type: 'local' | 'git' | 'central';
  source_ref?: string;
  central_path?: string;
  created_at: number;
  updated_at: number;
  files: SkillFileEntry[];
  metadata?: SkillMetadata;
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
  onDelete: (skillId: string) => void;
  formatRelative: (timestamp: number) => string;
}

export interface SkillDetailProps {
  skill: Skill | null;
  open: boolean;
  onClose: () => void;
  getFileContent: (skillId: string, filePath: string) => Promise<string | null>;
}

export interface UploadModalProps {
  open: boolean;
  onCancel: () => void;
  onUpload: (name: string, version: string, files: File[]) => Promise<boolean>;
  loading?: boolean;
}
