/**
 * Dataset artifact (knowledge compilation) — types returned by the
 * `/api/v1/datasets/<id>/artifacts*` endpoints.
 */

export interface ArtifactListItem {
  slug: string;
  title: string;
  page_type: string;
}

export interface ArtifactListResponse {
  total: number;
  items: ArtifactListItem[];
}

export interface ArtifactPage {
  slug: string;
  title: string;
  page_type: string;
  /** Pre-rendered markdown (links already in clickable form). */
  content_md_rendered: string;
  /** Optional standalone summary; the writer currently inlines it into the
   *  content body, so this is reserved for future expansion and may be empty. */
  summary?: string;
  entity_names?: string[];
  outlinks?: string[];
  related_kb_pages?: string[];
  source_chunk_ids?: string[];
  source_doc_ids?: string[];
}

export interface HasAnyArtifactResponse {
  has: boolean;
}

/**
 * Page-derived graph payload. Materialized by the task handler at the
 * end of ``_run_artifact`` and stored as a single ES row under
 * ``compile_kwd="artifact_page_graph"``. Each entity is one artifact
 * page; relations are slug-to-slug edges derived from ``outlinks``.
 */
export interface ArtifactGraphEntity {
  /** Stable id (``<page_type>/<tail>``). Used as the node id. */
  slug: string;
  /** Human-readable label (the page title). */
  name: string;
  /** Page type (``entity`` | ``concept`` | ``topic``). */
  type?: string;
  /** Page summary — shown in the node tooltip. */
  description?: string;
  /** Entity names covered by the page. */
  aliases?: string[];
  /**
   * Outlink count for this page — set by the backend writer.
   * Drives node visual size on the canvas.
   */
  weight?: number;
  /** Carried for legacy payloads; the new schema doesn't use it. */
  mention_count?: number;
}

export interface ArtifactGraphRelation {
  /** Source slug. */
  from: string;
  /** Target slug. */
  to: string;
  type?: string;
}

export interface ArtifactGraphResponse {
  entities: ArtifactGraphEntity[];
  relations: ArtifactGraphRelation[];
}
