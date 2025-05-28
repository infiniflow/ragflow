export interface ISchedule {
  id: string;
  tenant_id: string;
  canvas_id: string;
  name: string;
  description?: string;
  frequency_type: 'once' | 'daily' | 'weekly' | 'monthly';
  cron_expression?: string;
  execute_time?: string;
  execute_date?: string;
  days_of_week: number[];
  day_of_month?: number;
  enabled: boolean;
  next_run_time?: number;
  last_run_time?: number;
  run_count: number;
  input_params: Record<string, any>;
  created_by: string;
  status: string;
  create_time?: string;
  update_time?: string;
}

export interface IFrequencyOption {
  value: string;
  label: string;
  description: string;
  required_fields: string[];
}

export interface IFrequencyOptions {
  frequency_types: IFrequencyOption[];
  days_of_week: Array<{ value: number; label: string }>;
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
  extends Partial<ICreateScheduleRequest> {
  enabled?: boolean;
}
