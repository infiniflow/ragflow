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

export interface ISystemStatus {
  es: Es;
  storage: Storage;
  database: Database;
  redis: Redis;
  task_executor: {
    error?: string;
    status: string;
    elapsed?: TaskExecutorElapsed;
  };
}

interface Redis {
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

interface Es {
  status: string;
  elapsed: number;
  error: string;
  number_of_nodes: number;
  active_shards: number;
}
