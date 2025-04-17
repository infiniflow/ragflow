export interface IUserInfo {
  access_token: string;
  avatar?: any;
  color_schema: string;
  create_date: string;
  create_time: number;
  email: string;
  id: string;
  is_active: string;
  is_anonymous: string;
  is_authenticated: string;
  is_superuser: boolean;
  language: string;
  last_login_time: string;
  login_channel: string;
  nickname: string;
  password: string;
  status: string;
  update_date: string;
  update_time: number;
}

export type TaskExecutorElapsed = Record<string, number[]>;

export interface TaskExecutorHeartbeatItem {
  now: number;
  lag: number;
  pending: number;
  done: number;
  failed: number;
  [key: string]: any;
}

export interface ISystemStatus {
  doc_engine: {
    status: string;
    elapsed: string;
    [key: string]: any;
  };
  storage: {
    status: string;
    storage: string;
    elapsed: string;
    [key: string]: any;
  };
  database: {
    status: string;
    database: string;
    elapsed: string;
    [key: string]: any;
  };
  redis: {
    status: string;
    elapsed: string;
    [key: string]: any;
  };
  task_executor_heartbeats: Record<string, TaskExecutorHeartbeatItem[]>;
}

export interface Redis {
  status: string;
  elapsed: number;
  error: string;
  pending: number;
}

export interface Storage {
  status: string;
  elapsed: number;
  error: string;
}

export interface Database {
  status: string;
  elapsed: number;
  error: string;
}

export interface Es {
  status: string;
  elapsed: number;
  error: string;
  number_of_nodes: number;
  active_shards: number;
}

export interface ITenantUser {
  id: string;
  avatar: string;
  delta_seconds: number;
  email: string;
  is_active: string;
  is_anonymous: string;
  is_authenticated: string;
  is_superuser: boolean;
  nickname: string;
  role: string;
  status: string;
  update_date: string;
  user_id: string;
}

export interface ITenant {
  avatar: string;
  delta_seconds: number;
  email: string;
  nickname: string;
  role: string;
  tenant_id: string;
  update_date: string;
}
