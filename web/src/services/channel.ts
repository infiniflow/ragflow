import authorizationUtil from '@/utils/authorization-util';

const API_BASE = '/api/v1/channels';

function authHeaders(): HeadersInit {
  return {
    'Content-Type': 'application/json',
    Authorization: authorizationUtil.getAuthorization() ?? '',
  };
}

export interface IChannel {
  id: string;
  name: string;
  channel: string;
  dialog_id: string;
  status: 'enabled' | 'disabled';
  config: Record<string, unknown>;
  create_time?: number;
  update_time?: number;
}

export interface IChannelPayload {
  name: string;
  channel: string;
  dialog_id: string;
  config: Record<string, unknown>;
  status?: 'enabled' | 'disabled';
}

async function handleResponse<T>(res: Response): Promise<T> {
  const data = await res.json();
  if (!res.ok || data.code !== 0) {
    throw new Error(data.message ?? `HTTP ${res.status}`);
  }
  return data.data as T;
}

export async function getChannels(): Promise<IChannel[]> {
  const res = await fetch(API_BASE, { headers: authHeaders() });
  return handleResponse<IChannel[]>(res);
}

export async function createChannel(
  payload: IChannelPayload,
): Promise<IChannel> {
  const res = await fetch(API_BASE, {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify(payload),
  });
  return handleResponse<IChannel>(res);
}

export async function updateChannel(
  id: string,
  payload: Partial<IChannelPayload>,
): Promise<IChannel> {
  const res = await fetch(`${API_BASE}/${id}`, {
    method: 'PUT',
    headers: authHeaders(),
    body: JSON.stringify(payload),
  });
  return handleResponse<IChannel>(res);
}

export async function deleteChannel(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/${id}`, {
    method: 'DELETE',
    headers: authHeaders(),
  });
  return handleResponse<void>(res);
}

export interface IChannelFieldSchema {
  name: string;
  label: string;
  type: 'text' | 'password' | 'select';
  required: boolean;
  options?: string[];
}

export async function getChannelInfo(
  type: string,
): Promise<IChannelFieldSchema[]> {
  const res = await fetch(`${API_BASE}/info?type=${encodeURIComponent(type)}`, {
    headers: authHeaders(),
  });
  return handleResponse<IChannelFieldSchema[]>(res);
}
