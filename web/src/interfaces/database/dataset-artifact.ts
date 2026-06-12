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
 * REDUCE-phase graph payload — entity-centric only. The backend trims
 * concepts/claims and ``chunk_ids`` source attribution before
 * returning, so the canvas just gets nodes + edges to render.
 */
export interface ArtifactGraphEntity {
  name: string;
  type?: string;
  aliases?: string[];
  mention_count?: number;
}

export interface ArtifactGraphRelation {
  from: string;
  to: string;
  type?: string;
}

export interface ArtifactGraphResponse {
  entities: ArtifactGraphEntity[];
  relations: ArtifactGraphRelation[];
}
