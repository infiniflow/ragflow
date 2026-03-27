// Skill types for Skills Hub

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
}

export interface SkillMetadata {
  name?: string;
  description?: string;
  author?: string;
  version?: string;
  tools?: string[];
  tags?: string[];
  [key: string]: any;
}

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
