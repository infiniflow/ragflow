export interface IngestionEventItem {
  id: number;
  ts: string;
  event_type: number;
  component: string;
  phase: number;
  message: string;
}

export interface IngestionProgressResponse {
  status: string;
  phase?: string;
  stage?: string;
  unit?: string;
  current: number;
  total: number;
  percent: number;
  started_at?: string;
  duration_s: number;
  updated_at?: string;
  chunk_num: number;
  fallback: boolean;
}
