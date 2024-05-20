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

export interface ISystemStatus {
  es: Es;
  minio: Minio;
  mysql: Minio;
  redis: Redis;
}

interface Redis {
  status: string;
  elapsed: number;
  error: string;
  pending: number;
}

export interface Minio {
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
