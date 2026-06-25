/**
 * Knowledge compilation template — data shapes returned by the backend.
 *
 * The template body stored in `config_json` mirrors the YAML built-ins
 * served by `GET /v1/compilation_templates/builtins`; both are normalized
 * to {@link CompilationTemplateConfig} on the wire so the editor doesn't
 * have to parse YAML on the client.
 */

export const COMPILATION_TEMPLATE_KINDS = [
  'page_index',
  'timeline',
  'knowledge_graph',
  'artifacts',
  'tree',
  'empty',
] as const;

export type CompilationTemplateKind =
  (typeof COMPILATION_TEMPLATE_KINDS)[number];

export interface CompilationEntityField {
  type: string;
  description: string;
  rule: string;
}

export interface CompilationRelationField {
  type: string;
  description: string;
  rule: string;
}

export interface CompilationClaimField {
  statement: string;
  subject: string;
}

export interface CompilationConceptField {
  term: string;
  definition_excerpt: string;
}

export interface CompilationEntitySection {
  description: string;
  fields: CompilationEntityField[];
}

export interface CompilationRelationSection {
  description: string;
  fields: CompilationRelationField[];
}

export interface CompilationClaimSection {
  fields: CompilationClaimField[];
}

export interface CompilationConceptSection {
  fields: CompilationConceptField[];
}

export interface CompilationTemplateConfig {
  kind: CompilationTemplateKind;
  /** Chat model id used when this template runs (MAP/document-structure
   *  compile). Server-side lazy-fill seeds it from the tenant default
   *  when older templates open in the edit panel. */
  llm_id?: string;
  entity: CompilationEntitySection;
  relation: CompilationRelationSection;
  /** Present only when {@link kind} === 'artifacts'. */
  claim?: CompilationClaimSection;
  /** Present only when {@link kind} === 'artifacts'. */
  concept?: CompilationConceptSection;
  /** Present only when {@link kind} === 'artifacts'.
   *  Override for the page-structure section of the REFINE writer
   *  prompt. Empty / missing → backend falls back to its built-in
   *  ``ARTIFACT_TEMPLATE_EXAMPLE``. */
  example?: string;
  /** Present only when {@link kind} === 'tree'. RAPTOR-style knobs for
   *  the recursive summarization tree. */
  raptor?: CompilationRaptorSection;
  global_rules: string;
}

export interface CompilationRaptorSection {
  prompt: string;
  max_token: number;
  threshold: number;
}

export interface CompilationTemplate {
  id: string;
  name: string;
  description?: string;
  kind: CompilationTemplateKind;
  config: CompilationTemplateConfig;
  create_time?: number;
  update_time?: number;
}

export interface CompilationTemplateListResponse {
  total: number;
  templates: CompilationTemplate[];
}

/**
 * Same shape as {@link CompilationTemplate} but without persistence
 * metadata — used for the read-only built-in YAML defaults.
 */
export interface BuiltinCompilationTemplate {
  id: string;
  kind: CompilationTemplateKind;
  display_name: string;
  description?: string;
  config: CompilationTemplateConfig;
}
