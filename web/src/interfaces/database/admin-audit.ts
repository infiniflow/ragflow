export interface AdminOperation {
  id: string;
  event_time: string;
  actor_id: string;
  actor_email: string;
  target_id: string;
  target_email: string;
  action: string;
  api_path: string;
  ip_address: string;
  result: 'started' | 'success' | 'failure' | 'unknown';
  http_status: number;
  result_code: number;
  duration_ms: number;
}
