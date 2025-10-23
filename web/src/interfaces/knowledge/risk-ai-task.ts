export type RiskAITaskStatus = 'pending' | 'running' | 'success' | 'failed';

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
}
