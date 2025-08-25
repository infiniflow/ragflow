export interface ISchedule {
  id: string;
  tenant_id: string;
  canvas_id: string;
  name: string;
  description?: string;
  frequency_type: 'once' | 'daily' | 'weekly' | 'monthly';
  execute_time?: string;
  execute_date?: string;
  days_of_week?: number[];
  day_of_month?: number;
  enabled: boolean;
  input_params?: Record<string, any>;
  created_by: string;
  status: string;
  create_time: number;
  update_time: number;
  canvas_title?: string;
}

export interface IScheduleRun {
  id: string;
  schedule_id: string;
  started_at: Date;
  finished_at?: Date;
  success?: boolean;
  error_message?: string;
  conversation_id?: string;
}

export interface IScheduleStats {
  total_runs: number;
  successful_runs: number;
  failed_runs: number;
  last_successful_run?: IScheduleRun;
  is_currently_running: boolean;
}

export interface IFrequencyOptions {
  frequency_types: Array<{
    value: string;
    label: string;
    description: string;
    required_fields: string[];
  }>;
  days_of_week: Array<{
    value: number;
    label: string;
  }>;
  time_format: string;
  date_format: string;
}

export interface ICreateScheduleRequest {
  canvas_id: string;
  name: string;
  description?: string;
  frequency_type: string;
  execute_time?: string;
  execute_date?: string;
  days_of_week?: number[];
  day_of_month?: number;
  input_params?: Record<string, any>;
}

export interface IUpdateScheduleRequest
  extends Omit<ICreateScheduleRequest, 'canvas_id'> {
  id: string;
}
