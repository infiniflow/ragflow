export interface IngestionEventItem {
  id: number;
  ts: string;
  event_type: number;
  component: string;
  phase: number;
  message: string;
}

export interface IngestionMessagesResponse {
  run_count: number;
  items: IngestionEventItem[];
  oldest_id?: number;
  newest_id?: number;
  has_more_before: boolean;
  has_more_after: boolean;
  terminal: boolean;
}

export interface IngestionMessageParams {
  limit?: number;
  after_id?: number;
  before_id?: number;
}
