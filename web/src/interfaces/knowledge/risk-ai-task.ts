export type RiskAITaskStatus = 'pending' | 'running' | 'success' | 'failed';

export interface IRiskAITaskRowStatus {
  pending: number;
  running: number;
  success: number;
  failed: number;
}

export interface IRiskAITaskFailedRow {
  row_index: number;
  error_msg: string;
  payload: Record<string, any>;
}

export interface IRiskAITask {
  id: string;
  kb_id: string;
  status: RiskAITaskStatus;
  progress?: number;
  total_rows?: number;
  processed_rows?: number;
  failed_rows?: number;
  result_location?: string;
  download_url?: string;
  error_msg?: string;
  params?: Record<string, any>;
  created_by: string;
  create_time?: number;
  update_time?: number;
  row_status_counts?: IRiskAITaskRowStatus;
  failed_rows_detail?: IRiskAITaskFailedRow[];
}
